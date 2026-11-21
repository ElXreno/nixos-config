package hook

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mem/internal/env"
	"mem/internal/store"
)

const maxGitConfig = 1 << 20

func cwdOf(e env.Env, in Input) string {
	if in.Cwd != "" {
		return in.Cwd
	}
	if e.Getwd != nil {
		if wd, err := e.Getwd(); err == nil {
			return wd
		}
	}
	return ""
}

func gitRoot(dir string) string {
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		dot := filepath.Join(d, ".git")
		if fi, err := os.Stat(dot); err == nil {
			if fi.IsDir() {
				return d
			}
			if main := worktreeMain(dot); main != "" {
				return main
			}
			return d
		}
		if parent := filepath.Dir(d); parent == d {
			return ""
		}
	}
}

func worktreeMain(dotGit string) string {
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return ""
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(dotGit), gitdir)
	}
	gitdir = filepath.ToSlash(filepath.Clean(gitdir))
	i := strings.LastIndex(gitdir, "/.git/worktrees/")
	if i <= 0 {
		return ""
	}
	return filepath.FromSlash(gitdir[:i])
}

var scpRemote = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]{2,}):(.+)$`)

func projectFromRemote(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	var path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "file" || u.Host == "" {
			return "", false
		}
		path = u.Path
	default:
		m := scpRemote.FindStringSubmatch(raw)
		if m == nil {
			return "", false
		}
		path = m[2]
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	var segs []string
	for _, seg := range strings.Split(path, "/") {
		switch seg {
		case "":
			continue
		case ".", "..":
			return "", false
		}
		segs = append(segs, strings.ToLower(seg))
	}
	return validProject(strings.Join(segs, "/"))
}

func validProject(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if _, err := store.NormalizeScope(store.ProjectScope(name)); err != nil {
		return "", false
	}
	return name, true
}

func gitConfigPath(root string) string {
	dot := filepath.Join(root, ".git")
	fi, err := os.Stat(dot)
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return filepath.Join(dot, "config")
	}
	data, err := os.ReadFile(dot)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return ""
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	return filepath.Join(gitdir, "config")
}

func configValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' {
		if end := strings.IndexByte(v[1:], '"'); end >= 0 {
			return v[1 : end+1]
		}
	}
	if i := strings.IndexAny(v, "#;"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func originURL(root string) string {
	path := gitConfigPath(root)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > maxGitConfig {
		return ""
	}
	inOrigin := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			head := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			name, sub, quoted := strings.Cut(head, " ")
			inOrigin = quoted && strings.EqualFold(name, "remote") && strings.TrimSpace(sub) == `"origin"` || strings.EqualFold(head, "remote.origin")
			continue
		}
		if !inOrigin {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(k), "url") {
			return configValue(v)
		}
	}
	return ""
}

func pathProject(e env.Env, root string) string {
	name := strings.TrimPrefix(filepath.ToSlash(root), "/")
	if home := filepath.Clean(e.Home()); home != "" && home != "." && home != string(filepath.Separator) {
		if root == home {
			name = "~"
		} else if rel, ok := strings.CutPrefix(root, home+string(filepath.Separator)); ok {
			name = "~/" + filepath.ToSlash(rel)
		}
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', strings.ContainsRune("_.+@~-/", r):
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}
		return '-'
	}, name)
	out, _ := validProject(name)
	return out
}

func project(e env.Env, in Input) string {
	root := gitRoot(cwdOf(e, in))
	if root == "" || root == string(filepath.Separator) {
		return ""
	}
	if name, ok := projectFromRemote(originURL(root)); ok {
		return name
	}
	return pathProject(e, root)
}

func here(e env.Env, in Input, cfg config) store.Here {
	h := store.Here{StrictHost: cfg.strictHost, StrictProject: cfg.strict}
	if p := project(e, in); p != "" {
		h.Project = store.ProjectScope(p)
	}
	if host := e.Host(); host != "" {
		h.Host = store.HostScope(host)
	}
	return h
}

func localScopes(e env.Env, in Input) []string {
	var out []string
	if p := project(e, in); p != "" {
		out = append(out, store.ProjectScope(p))
	}
	if h := e.Host(); h != "" {
		out = append(out, store.HostScope(h))
	}
	return out
}

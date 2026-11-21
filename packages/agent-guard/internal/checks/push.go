package checks

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"agent-guard/internal/shell"
	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

var gitProgram = "git"

var sshProgram = "ssh"

var sshArgs []string

const gitTimeout = 3 * time.Second

const pushTail = " Yes only if the user asked for this push; otherwise commit locally and leave the push to the user."

const unknownDir = "the directory it runs in is only known when the command runs"

const remotePushMsg = "git push on another host or in a container: agent-guard cannot read the remotes there, so it may publish to a protected repository." + pushTail

var (
	schemeRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	assignWord = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
)

func ValidRepoPattern(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.New("empty pattern")
	}
	_, err := path.Match(strings.ToLower(repoKey(p)), "")
	return err
}

func scpLike(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon <= 0 {
		return false
	}
	slash := strings.IndexByte(s, '/')
	return slash < 0 || colon < slash
}

func urlLike(s string) bool {
	return schemeRe.MatchString(s) || scpLike(s) || strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") || strings.HasPrefix(s, "~")
}

func repoKey(u string) string {
	s := strings.TrimSpace(u)
	var host, p string
	switch {
	case schemeRe.MatchString(s):
		rest := s[strings.Index(s, "://")+3:]
		var found bool
		host, p, found = strings.Cut(rest, "/")
		if !found {
			p = ""
		}
		p = "/" + p
		if i := strings.LastIndexByte(host, '@'); i >= 0 {
			host = host[i+1:]
		}
		if strings.HasPrefix(host, "[") {
			if j := strings.IndexByte(host, ']'); j >= 0 {
				host = host[:j+1]
			}
		} else if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
	case scpLike(s):
		i := strings.IndexByte(s, ':')
		host, p = s[:i], "/"+s[i+1:]
		if j := strings.LastIndexByte(host, '@'); j >= 0 {
			host = host[j+1:]
		}
	default:
		return filepath.Clean(s)
	}
	p = strings.TrimSuffix(path.Clean(p), "/")
	p = strings.TrimSuffix(strings.TrimSuffix(p, ".git"), "/")
	return host + p
}

func keyMatch(pat, key string) bool {
	if ok, _ := path.Match(pat, key); ok {
		return true
	}
	ph, pp, ok := strings.Cut(pat, "/")
	kh, kp, ok2 := strings.Cut(key, "/")
	if !ok || !ok2 || ph == "" || kh == "" {
		return false
	}
	if sub, _ := path.Match("*."+ph, kh); !sub {
		return false
	}
	ok, _ = path.Match(pp, kp)
	return ok
}

func (cfg *Settings) protectedRepoKey(key string) bool {
	lk := strings.ToLower(key)
	for _, pat := range cfg.ProtectedRepos {
		if keyMatch(strings.ToLower(repoKey(pat)), lk) {
			return true
		}
	}
	return false
}

func (cfg *Settings) protectedKey(url string) (string, bool) {
	key := repoKey(url)
	return key, cfg.protectedRepoKey(key)
}

func sshTarget(u string) (host, rest string, ok bool) {
	s := strings.TrimSpace(u)
	if m := schemeRe.FindString(s); m != "" {
		switch strings.ToLower(strings.TrimSuffix(m, "://")) {
		case "ssh", "git+ssh", "ssh+git":
		default:
			return "", "", false
		}
	} else if !scpLike(s) {
		return "", "", false
	}
	host, rest, _ = strings.Cut(repoKey(s), "/")
	if host == "" || strings.HasPrefix(host, "-") || strings.HasPrefix(host, "[") {
		return "", "", false
	}
	return host, "/" + rest, true
}

func (g *gitReader) sshHostName(host string) string {
	if h, ok := g.hosts[host]; ok {
		return h
	}
	g.hosts[host] = ""
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, sshProgram, append(append([]string{}, sshArgs...), "-G", "--", host)...).Output()
	if err != nil {
		return ""
	}
	for line := range strings.Lines(string(out)) {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "hostname "); ok {
			g.hosts[host] = strings.ToLower(strings.TrimSpace(name))
			break
		}
	}
	return g.hosts[host]
}

func (g *gitReader) protectedKey(cfg *Settings, url string) (string, bool) {
	if key, ok := cfg.protectedKey(url); ok {
		return key, true
	}
	host, rest, ok := sshTarget(url)
	if !ok {
		return "", false
	}
	real := g.sshHostName(host)
	if real == "" || real == strings.ToLower(host) {
		return "", false
	}
	key := real + rest
	return key, cfg.protectedRepoKey(key)
}

func unresolved(s string) bool { return strings.ContainsAny(s, "$`") }

type gitLoc struct {
	dir    string
	gitDir string
	config []string
	env    []string
}

func (l gitLoc) args(extra ...string) []string {
	var out []string
	if l.dir != "" {
		out = append(out, "-C", l.dir)
	}
	if l.gitDir != "" {
		out = append(out, "--git-dir="+l.gitDir)
	}
	for _, c := range l.config {
		out = append(out, "-c", c)
	}
	return append(out, extra...)
}

type gitState int

const (
	gitOK gitState = iota
	gitNotRepo
	gitFailed
)

type remote struct {
	fetch, push []string
}

type gitRead struct {
	state   gitState
	err     error
	remotes map[string]*remote
	order   []string
}

type gitReader struct {
	cache map[string]*gitRead
	hosts map[string]string
}

func newGitReader() *gitReader {
	return &gitReader{cache: map[string]*gitRead{}, hosts: map[string]string{}}
}

func runGit(l gitLoc, args ...string) (string, gitState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, gitProgram, l.args(args...)...)
	c.Env = append(append(os.Environ(), l.env...), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	out, err := c.Output()
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return "", gitFailed, ctx.Err()
	case errors.As(err, &exit):
		return "", gitNotRepo, err
	case err != nil:
		return "", gitFailed, err
	}
	return string(out), gitOK, nil
}

func (g *gitReader) read(l gitLoc) *gitRead {
	key := strings.Join(append(l.args(), l.env...), "\x00")
	if r, ok := g.cache[key]; ok {
		return r
	}
	r := &gitRead{remotes: map[string]*remote{}}
	g.cache[key] = r
	out, state, err := runGit(l, "remote", "-v")
	r.state, r.err = state, err
	if state != gitOK {
		return r
	}
	for line := range strings.Lines(out) {
		name, rest, ok := strings.Cut(strings.TrimRight(line, "\n"), "\t")
		if !ok {
			continue
		}
		rm := r.remotes[name]
		if rm == nil {
			rm = &remote{}
			r.remotes[name] = rm
			r.order = append(r.order, name)
		}
		switch {
		case strings.HasSuffix(rest, " (push)"):
			rm.push = append(rm.push, strings.TrimSuffix(rest, " (push)"))
		case strings.HasSuffix(rest, " (fetch)"):
			rm.fetch = append(rm.fetch, strings.TrimSuffix(rest, " (fetch)"))
		}
	}
	return r
}

func (g *gitReader) defaultRemote(l gitLoc, r *gitRead) string {
	out, state, _ := runGit(l, "for-each-ref", "--format=%(HEAD)%(push:remotename)", "refs/heads/")
	if state == gitOK {
		for line := range strings.Lines(out) {
			if name, ok := strings.CutPrefix(strings.TrimSpace(line), "*"); ok && name != "" {
				return name
			}
		}
	}
	if _, ok := r.remotes["origin"]; !ok && len(r.order) == 1 {
		return r.order[0]
	}
	return "origin"
}

func expandURL(l gitLoc, url string) string {
	out, state, _ := runGit(l, "ls-remote", "--get-url", url)
	if state != gitOK {
		return ""
	}
	return strings.TrimSpace(out)
}

func (g *gitReader) anyProtected(cfg *Settings, r *gitRead) (string, bool) {
	for _, name := range r.order {
		rm := r.remotes[name]
		for _, u := range append(append([]string{}, rm.fetch...), rm.push...) {
			if key, ok := g.protectedKey(cfg, u); ok {
				return key, true
			}
		}
	}
	return "", false
}

type pushArgs struct {
	repo   string
	dryRun bool
}

func parsePush(args []string) pushArgs {
	var out pushArgs
	var positional []string
	repoOpt := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case a == "--dry-run":
			out.dryRun = true
		case len(a) >= len("--no-dr") && strings.HasPrefix("--no-dry-run", a):
			out.dryRun = false
		case a == "--repo" || a == "--rep" || a == "--push-option" || a == "--receive-pack" || a == "--exec":
			if i+1 < len(args) {
				if strings.HasPrefix(a, "--rep") {
					repoOpt = args[i+1]
				}
				i++
			}
		case strings.HasPrefix(a, "--repo=") || strings.HasPrefix(a, "--rep="):
			_, repoOpt, _ = strings.Cut(a, "=")
		case strings.HasPrefix(a, "--"):
		case len(a) > 1 && a[0] == '-':
			for k := 1; k < len(a); k++ {
				if a[k] == 'n' {
					out.dryRun = true
				}
				if a[k] == 'o' {
					if k == len(a)-1 {
						i++
					}
					break
				}
			}
		default:
			positional = append(positional, a)
		}
	}
	out.repo = repoOpt
	if len(positional) > 0 {
		out.repo = positional[0]
	}
	return out
}

var argFeeders = map[string]bool{"xargs": true, "parallel": true}

func wrapperSettings(cmd shell.Command) (env map[string]string, chdir string, fed bool) {
	n := len(cmd.Argv) - len(cmd.Args()) - 1
	if n <= 0 || n > len(cmd.Argv) {
		return nil, "", false
	}
	words := cmd.Argv[:n]
	env = map[string]string{}
	for i := 0; i < len(words); i++ {
		if m := assignWord.FindStringSubmatch(words[i]); m != nil {
			env[m[1]] = m[2]
			continue
		}
		if argFeeders[filepath.Base(words[i])] {
			fed = true
		}
		if filepath.Base(words[i]) != "env" {
			continue
		}
		for i+1 < len(words) && strings.HasPrefix(words[i+1], "-") {
			i++
			o := words[i]
			switch {
			case o == "-C" || o == "--chdir":
				if i+1 < len(words) {
					i++
					chdir = words[i]
				}
			case strings.HasPrefix(o, "--chdir="):
				chdir = strings.TrimPrefix(o, "--chdir=")
			case strings.HasPrefix(o, "-C"):
				chdir = o[2:]
			case o == "-u" || o == "--unset":
				i++
			}
		}
	}
	return env, chdir, fed
}

func gitConfigEnv(envs ...map[string]string) (out []string, exact bool) {
	merged := map[string]string{}
	for _, e := range envs {
		for k, v := range e {
			if strings.HasPrefix(k, "GIT_CONFIG") {
				merged[k] = v
			}
		}
	}
	exact = true
	for _, k := range slices.Sorted(maps.Keys(merged)) {
		v := merged[k]
		if n, ok := strings.CutPrefix(k, "GIT_CONFIG_VALUE_"); ok {
			exact = exact && configExact(merged["GIT_CONFIG_KEY_"+n]+"="+v)
		} else if unresolved(v) {
			exact = false
		}
		out = append(out, k+"="+v)
	}
	return out, exact
}

var destinationKeys = []string{"url.", "remote.", "branch.", "push.", "include.", "includeif."}

func configExact(kv string) bool {
	key, _, _ := strings.Cut(kv, "=")
	if unresolved(key) {
		return false
	}
	if !unresolved(kv) {
		return true
	}
	key = strings.ToLower(key)
	return !slices.ContainsFunc(destinationKeys, func(p string) bool { return strings.HasPrefix(key, p) })
}

func (cfg *Settings) gitLocation(cmd shell.Command, globals []string, cwd string, known bool) (gitLoc, bool) {
	exact := true
	move := func(base, p string) string {
		if p == "" {
			return base
		}
		r := ""
		if !unresolved(p) {
			r = cfg.resolve(p, base)
		}
		if r == "" {
			known = false
			return base
		}
		if filepath.IsAbs(cfg.expand(p)) {
			known = true
		}
		return r
	}
	env, chdir, fed := wrapperSettings(cmd)
	if fed {
		exact = false
	}
	dir := move(cwd, chdir)
	loc := gitLoc{}
	var envExact bool
	loc.env, envExact = gitConfigEnv(cmd.Env, env)
	exact = exact && envExact
	gitDir, gitDirSet := "", false
	for i := 0; i < len(globals); i++ {
		g := globals[i]
		val := ""
		if i+1 < len(globals) {
			val = globals[i+1]
		}
		switch {
		case g == "-C":
			dir = move(dir, val)
			i++
		case g == "-c":
			exact = exact && configExact(val)
			loc.config = append(loc.config, val)
			i++
		case g == "--git-dir":
			gitDir, gitDirSet = val, true
			i++
		case strings.HasPrefix(g, "--git-dir="):
			gitDir, gitDirSet = strings.TrimPrefix(g, "--git-dir="), true
		case g == "--work-tree" || g == "--namespace" || g == "--exec-path":
			i++
		}
	}
	if !gitDirSet {
		for _, e := range []map[string]string{cmd.Env, env} {
			if v, ok := e["GIT_DIR"]; ok {
				gitDir, gitDirSet = v, true
			}
		}
	}
	loc.dir = dir
	if gitDirSet && gitDir != "" {
		if d := cfg.resolve(gitDir, dir); d != "" && !unresolved(gitDir) {
			loc.gitDir = d
		} else {
			exact = false
		}
	}
	return loc, exact && known
}

func protectedMsg(key string) string {
	return "git push to " + key + ", a protected repository: the push publishes there." + pushTail
}

func unresolvedMsg(key, why string) string {
	return "git push from a checkout of " + key + ", a protected repository, and " + why + ", so it may publish there." + pushTail
}

func unknownMsg(why string) string {
	return "git push, and " + why + ", so it may publish to a protected repository." + pushTail
}

func failedMsg(err error) string {
	return "git push, and agent-guard could not read the remotes (" + text.Clip(err.Error(), 120) + "), so it may publish to a protected repository." + pushTail
}

func under(p, root string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}

func (cfg *Settings) cloneOf(cmd shell.Command, cwd string) (dir, url string, ok bool) {
	sub, rest, isGit := gitCall(cmd)
	if !isGit || sub != "clone" || cmd.Kind == shell.Remote {
		return "", "", false
	}
	pos := optPositionals(rest, "bojcu", cloneArgOpts)
	switch {
	case len(pos) >= 2:
		dir = pos[1]
	case len(pos) == 1:
		dir = strings.TrimSuffix(urlBase(pos[0]), ".git")
	default:
		return "", "", false
	}
	dir = cfg.resolve(dir, cwd)
	return dir, pos[0], dir != ""
}

func (cfg *Settings) pushVerdict(cmd shell.Command, cwd string, known bool, g *gitReader, clones map[string]string) *verdict.Verdict {
	globals, sub, args, ok := gitInvocation(cmd)
	if !ok || sub != "push" {
		return nil
	}
	p := parsePush(args)
	if p.dryRun {
		return nil
	}
	literal := p.repo != "" && !unresolved(p.repo) && urlLike(p.repo)
	if literal {
		if key, ok := g.protectedKey(cfg, p.repo); ok {
			return ask(protectedMsg(key))
		}
	}
	if cmd.Kind == shell.Remote {
		if literal {
			return nil
		}
		return ask(remotePushMsg)
	}
	loc, exact := cfg.gitLocation(cmd, globals, cwd, known)
	if loc.dir == "" && loc.gitDir == "" {
		if !exact && !literal {
			return ask(unknownMsg(unknownDir))
		}
		return nil
	}
	var urls []string
	if p.repo == "" || !urlLike(p.repo) {
		for d, u := range clones {
			if loc.gitDir == "" && under(loc.dir, d) {
				urls = append(urls, u)
			}
		}
	}
	r := g.read(loc)
	if r.state == gitFailed {
		return ask(failedMsg(r.err))
	}
	why, unknown := "", false
	switch {
	case literal:
		if u := expandURL(loc, p.repo); u != "" {
			urls = append(urls, u)
		}
	case p.repo != "" && unresolved(p.repo):
		why, unknown = "its destination "+text.Clip(p.repo, 80)+" is only known when the command runs", true
	case !exact:
		why, unknown = unknownDir, true
	case p.repo != "":
		if rm, ok := r.remotes[p.repo]; ok {
			urls = append(urls, rm.push...)
		} else if r.state == gitOK {
			why = "it names " + text.Clip(p.repo, 80) + ", which is no remote of this checkout"
		}
	case r.state == gitOK:
		name := g.defaultRemote(loc, r)
		if rm, ok := r.remotes[name]; ok {
			urls = append(urls, rm.push...)
		} else {
			why = "its default remote " + name + " is not configured"
		}
	}
	for _, u := range urls {
		if key, ok := g.protectedKey(cfg, u); ok {
			return ask(protectedMsg(key))
		}
	}
	if why == "" {
		return nil
	}
	if r.state == gitOK {
		if key, ok := g.anyProtected(cfg, r); ok {
			return ask(unresolvedMsg(key, why))
		}
	}
	if unknown {
		return ask(unknownMsg(why))
	}
	return nil
}

func gitPush(a *shell.Analysis, ctx Context, cfg *Settings) *verdict.Verdict {
	if len(cfg.ProtectedRepos) == 0 {
		return nil
	}
	g := newGitReader()
	clones := map[string]string{}
	var out *verdict.Verdict
	walkDirs(a, ctx.Cwd, cfg, func(cmd shell.Command, cwd string, known bool) {
		if out != nil {
			return
		}
		if dir, url, ok := cfg.cloneOf(cmd, cwd); ok {
			clones[dir] = url
			return
		}
		out = cfg.pushVerdict(cmd, cwd, known, g, clones)
	})
	return out
}

package checks

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

// Context is what the hook knows about the session that the command alone
// does not say: where it runs, which session it is, what day it is.
type Context struct {
	Cwd       string
	SessionID string
	Today     string
	// AssumeNew treats every target as not yet existing; the replay tool uses
	// it because the files of past sessions are long gone.
	AssumeNew bool
}

type target struct {
	path     string
	forceNew bool
}

var unresolvable = regexp.MustCompile("[$`*?\\[{]")

// location warns when a command creates files outside the places agreed for
// them. Existing files are somebody's deliberate work and are left alone.
func location(a *shell.Analysis, ctx Context, cfg *Settings) *verdict.Verdict {
	if ctx.Cwd == "" && !ctx.AssumeNew {
		return nil
	}
	var offenders []string
	root := ""
	walkTargets(a, ctx.Cwd, cfg, func(p, cwd string, t target) {
		if !ctx.AssumeNew && !t.forceNew && exists(p) {
			return
		}
		if r, ok := cfg.allowed(p, cwd); !ok {
			if len(offenders) == 0 {
				root = r
			}
			offenders = append(offenders, p)
		}
	})
	if len(offenders) == 0 {
		return nil
	}
	return advise(locationMsg(offenders, ctx, cfg, root, ctx.Cwd))
}

func (cfg *Settings) allowed(p, cwd string) (string, bool) {
	root := gitRootOf(filepath.Dir(p))
	if root == "" {
		root = gitRootOf(cwd)
	}
	return root, cfg.underAny(p, cfg.createRoots(root))
}

func walkTargets(a *shell.Analysis, cwd string, cfg *Settings, visit func(p, cwd string, t target)) {
	seen := map[string]bool{}
	walkDirs(a, cwd, cfg, func(cmd shell.Command, cwd string, _ bool) {
		if cmd.Kind == shell.Remote {
			return
		}
		for _, t := range createTargets(cmd, cwd, cfg) {
			p := cfg.resolve(t.path, cwd)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			visit(p, cwd, t)
		}
	})
}

type dirEntry struct {
	cwd     string
	unknown bool
}

type dirFrame struct {
	dirEntry
	stack []dirEntry
}

func (cfg *Settings) knownAfterCd(e dirEntry, args []string) bool {
	pos := positionals(args)
	switch {
	case len(pos) == 0:
		return true
	case pos[0] == "-" || cfg.resolve(pos[0], e.cwd) == "":
		return false
	}
	return !e.unknown || filepath.IsAbs(cfg.expand(pos[0]))
}

func walkDirs(a *shell.Analysis, cwd string, cfg *Settings, visit func(cmd shell.Command, cwd string, known bool)) {
	frames := []dirFrame{{dirEntry: dirEntry{cwd: cwd}}}
	for _, cmd := range a.Commands {
		d := max(cmd.Depth, 0)
		if d < len(frames)-1 {
			frames = frames[:d+1]
		}
		for len(frames) <= d {
			frames = append(frames, dirFrame{dirEntry: frames[len(frames)-1].dirEntry})
		}
		if cmd.Kind == shell.Remote {
			visit(cmd, "", true)
			continue
		}
		f := &frames[d]
		switch cmd.Head() {
		case "cd":
			f.unknown = !cfg.knownAfterCd(f.dirEntry, cmd.Args())
			f.cwd = cfg.changeDir(f.cwd, cmd.Args())
		case "pushd":
			pos := positionals(cmd.Args())
			switch {
			case len(pos) == 0 && len(f.stack) > 0:
				top := len(f.stack) - 1
				f.dirEntry, f.stack[top] = f.stack[top], f.dirEntry
			case len(pos) > 0 && !strings.HasPrefix(pos[0], "+"):
				f.stack = append(f.stack, f.dirEntry)
				f.unknown = !cfg.knownAfterCd(f.dirEntry, pos[:1])
				f.cwd = cfg.changeDir(f.cwd, pos[:1])
			case len(pos) > 0:
				f.unknown = true
			}
		case "popd":
			if n := len(f.stack); n > 0 && len(cmd.Args()) == 0 {
				f.dirEntry, f.stack = f.stack[n-1], f.stack[:n-1]
			}
		default:
			visit(cmd, f.cwd, !f.unknown)
		}
	}
}

// LocationForNewFile applies the same rule to a file tool: a new file outside
// the agreed places gets the same nudge.
func LocationForNewFile(path string, ctx Context, cfg *Settings) *verdict.Verdict {
	p := cfg.resolve(path, ctx.Cwd)
	if p == "" || (!ctx.AssumeNew && exists(p)) {
		return nil
	}
	root, ok := cfg.allowed(p, ctx.Cwd)
	if ok {
		return nil
	}
	v := advise(locationMsg([]string{p}, ctx, cfg, root, ctx.Cwd))
	v.Check = Location
	return v
}

func locationMsg(paths []string, ctx Context, cfg *Settings, gitRoot, cwd string) string {
	project := filepath.Base(gitRoot)
	if gitRoot == "" {
		project = filepath.Base(cwd)
	}
	if project == "" || project == "/" || project == "." {
		project = "misc"
	}
	sid := cfg.spec().SessionTag(ctx.SessionID)
	if sid == "" {
		sid = "0000"
	}
	day := ctx.Today
	if day == "" {
		day = "YYYY-MM-DD"
	}
	var b strings.Builder
	b.WriteString("New files outside the agreed scratch area: " + strings.Join(paths, ", ") + ".")
	repos := "repository files and clones under " + cfg.ReposRoot + " are fine where they are"
	switch {
	case cfg.ScratchBase != "":
		b.WriteString(" Session scratch lives under " + filepath.Join(cfg.ScratchBase, project, day+"-<topic>-"+sid) + "/ (mkdir -p it first, pick a short topic)")
		if cfg.ReposRoot != "" {
			b.WriteString("; " + repos)
		}
		b.WriteString(".")
	case cfg.ReposRoot != "":
		b.WriteString(" R" + repos[1:] + ".")
	}
	for i, r := range cfg.AvoidRoots {
		if r == cfg.Home {
			r = "$HOME"
		}
		if i == 0 {
			b.WriteString(" Not " + r)
		} else {
			b.WriteString(", not " + r)
		}
	}
	if len(cfg.AvoidRoots) > 0 {
		b.WriteString(".")
	}
	return b.String()
}

var pseudoRoots = []string{"/dev", "/proc", "/sys"}

func (cfg *Settings) createRoots(gitRoot string) []string {
	roots := append([]string{}, cfg.CreateRoots...)
	for _, r := range []string{cfg.ScratchBase, cfg.ReposRoot} {
		if r != "" {
			roots = append(roots, r)
		}
	}
	roots = append(roots, pseudoRoots...)
	if dir := cfg.spec().TaskDir(); dir != "" {
		roots = append(roots, dir)
	}
	if gitRoot != "" {
		roots = append(roots, gitRoot)
	}
	return roots
}

func (cfg *Settings) resolve(p, cwd string) string {
	if p == "" || unresolvable.MatchString(p) {
		return ""
	}
	p = cfg.expand(p)
	if !filepath.IsAbs(p) {
		if cwd == "" {
			return ""
		}
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

func (cfg *Settings) changeDir(cwd string, args []string) string {
	pos := positionals(args)
	if len(pos) == 0 {
		return cfg.Home
	}
	if pos[0] == "-" {
		return cwd
	}
	if p := cfg.resolve(pos[0], cwd); p != "" {
		return p
	}
	return cwd
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func GitRoot(dir string) string { return gitRootOf(dir) }

func gitRootOf(dir string) string {
	for dir != "" && dir != "/" && dir != "." {
		if exists(filepath.Join(dir, ".git")) {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

var (
	writeRedirs   = map[string]bool{">": true, ">>": true, ">|": true, "&>": true, "&>>": true}
	cloneArgOpts  = regexp.MustCompile(`^(-b|--branch|--depth|-o|--origin|-c|--config|--reference|--separate-git-dir|-j|--jobs|--template|-u|--upload-pack|--filter)$`)
	curlArgOpts   = regexp.MustCompile(`^(-H|-d|-X|-u|-A|-b|-c|-e|-F|-T|-x|-m|-w|-K|-E|-y|-Y|-z|--data|--header|--request|--user|--user-agent|--cookie|--cookie-jar|--referer|--form|--upload-file|--proxy|--max-time|--write-out|--config|--cert|--key|--cacert|--resolve|--connect-to|--retry|--retry-delay|--limit-rate|--output-dir)$`)
	installArgOps = regexp.MustCompile(`^(-m|-o|-g|--mode|--owner|--group|-t|--target-directory)$`)
	mkdirArgOpts  = regexp.MustCompile(`^--mode$`)
	rsyncArgOpts  = regexp.MustCompile(`^(--rsh|--rsync-path|--exclude|--include|--exclude-from|--include-from|--files-from|--log-file|--bwlimit|--timeout|--chmod|--chown)$`)
)

func createTargets(cmd shell.Command, cwd string, cfg *Settings) []target {
	var out []target
	add := func(p string) {
		if p != "" && !strings.HasPrefix(p, "&") && !strings.HasPrefix(p, "/dev/") {
			out = append(out, target{path: p})
		}
	}
	for _, r := range cmd.Redirects {
		if writeRedirs[r.Op] {
			add(r.Target)
		}
	}
	head := cmd.Head()
	args := cmd.Args()
	switch head {
	case "tee":
		for _, p := range optPositionals(args, "", nil) {
			add(p)
		}
	case "mkdir":
		for _, p := range optPositionals(args, "m", mkdirArgOpts) {
			add(p)
		}
	case "cp", "mv", "install":
		pos := optPositionals(args, "mogt", installArgOps)
		if head == "install" && contains(args, "-d", "--directory") {
			for _, p := range pos {
				add(p)
			}
			return out
		}
		if len(pos) >= 2 {
			for _, p := range intoDest(pos, cwd, cfg) {
				add(p)
			}
		}
	case "rsync", "scp":
		pos := optPositionals(args, "eiPpFlJoBSc", rsyncArgOpts)
		if len(pos) >= 2 && !strings.Contains(pos[len(pos)-1], ":") {
			for _, p := range intoDest(pos, cwd, cfg) {
				add(p)
			}
		}
	case "curl":
		url := ""
		for _, p := range optPositionals(args, "HdXuAbceFTxmwKEyYz", curlArgOpts) {
			if strings.Contains(p, "://") {
				url = p
				break
			}
		}
		for i, a := range args {
			cluster, short := shortCluster(a)
			switch {
			case (a == "--output" || (short && strings.HasSuffix(cluster, "o"))) && i+1 < len(args):
				add(args[i+1])
			case strings.HasPrefix(a, "--output="):
				add(strings.TrimPrefix(a, "--output="))
			case a == "--remote-name" || (short && strings.ContainsRune(cluster, 'O')):
				if url != "" {
					add(urlBase(url))
				}
			}
		}
	case "wget":
		url := ""
		prefix := ""
		named := false
		for i, a := range args {
			switch {
			case (a == "-O" || a == "--output-document") && i+1 < len(args):
				add(args[i+1])
				named = true
			case strings.HasPrefix(a, "--output-document="):
				add(strings.TrimPrefix(a, "--output-document="))
				named = true
			case (a == "-P" || a == "--directory-prefix") && i+1 < len(args):
				prefix = args[i+1]
			case strings.HasPrefix(a, "--directory-prefix="):
				prefix = strings.TrimPrefix(a, "--directory-prefix=")
			case strings.Contains(a, "://"):
				url = a
			}
		}
		if !named && url != "" {
			add(filepath.Join(prefix, urlBase(url)))
		}
	case "git":
		if sub, rest, ok := gitCall(cmd); ok && sub == "clone" {
			pos := optPositionals(rest, "bojcu", cloneArgOpts)
			switch {
			case len(pos) >= 2:
				add(pos[1])
			case len(pos) == 1:
				add(strings.TrimSuffix(urlBase(pos[0]), ".git"))
			}
		}
	case "nix":
		if len(args) > 0 && args[0] == "build" && !contains(args, "--no-link") {
			add(outLink(args))
		}
	case "nix-build":
		if !contains(args, "--no-out-link") {
			add(outLink(args))
		}
	case "mktemp":
		dir := ""
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch {
			case (a == "-p" || a == "--tmpdir") && i+1 < len(args):
				dir = args[i+1]
				i++
			case strings.HasPrefix(a, "--tmpdir="):
				dir = strings.TrimPrefix(a, "--tmpdir=")
			case !strings.HasPrefix(a, "-") && strings.Contains(a, "/"):
				dir = filepath.Dir(a)
			}
		}
		if dir == "" {
			dir = os.TempDir()
		}
		out = append(out, target{path: dir, forceNew: true})
	case "dd":
		for _, a := range args {
			if strings.HasPrefix(a, "of=") {
				add(strings.TrimPrefix(a, "of="))
			}
		}
	}
	interpreter := shell.IsInterpreter(head)
	if interpreter || (cmd.Heredoc != "" && !dataSinks[head]) {
		code := cmd.Heredoc
		for i, a := range args {
			if interpreter && (a == "-c" || a == "-e" || a == "--eval") && i+1 < len(args) {
				code = args[i+1]
			}
		}
		if code != "" {
			for _, p := range scriptWriteTargets(code) {
				add(p)
			}
		}
	}
	return out
}

func outLink(args []string) string {
	link := "result"
	for i, a := range args {
		switch {
		case (a == "-o" || a == "--out-link") && i+1 < len(args):
			link = args[i+1]
		case strings.HasPrefix(a, "--out-link="):
			link = strings.TrimPrefix(a, "--out-link=")
		}
	}
	return link
}

// intoDest expands `cp a b dir/` into the files that appear inside dir.
func intoDest(pos []string, cwd string, cfg *Settings) []string {
	dest := pos[len(pos)-1]
	srcs := pos[:len(pos)-1]
	resolved := cfg.resolve(dest, cwd)
	if len(srcs) > 1 || strings.HasSuffix(dest, "/") || (resolved != "" && isDir(resolved)) {
		var out []string
		for _, s := range srcs {
			out = append(out, filepath.Join(dest, filepath.Base(strings.TrimSuffix(s, "/"))))
		}
		return out
	}
	return []string{dest}
}

func urlBase(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = strings.TrimSuffix(u, "/")
	return filepath.Base(u)
}

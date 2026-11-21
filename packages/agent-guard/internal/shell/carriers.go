package shell

import (
	"regexp"
	"strings"
)

var shells = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true, "mksh": true, "ash": true, "fish": true,
}

var remoteHeads = map[string]bool{
	"ssh": true, "autossh": true, "docker": true, "podman": true, "nerdctl": true, "kubectl": true, "oc": true,
	"incus": true, "lxc": true, "nsenter": true, "distrobox": true, "toolbox": true, "machinectl": true, "chroot": true,
	"adb": true,
}

func firstWord(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return base(f[0])
}

// wrapperArgs lists, per wrapper, the short options that consume the next word.
var wrapperArgs = map[string]string{
	"sudo": "ugCUprDh", "env": "uC", "nice": "n", "ionice": "cnp", "chrt": "p", "taskset": "cp",
	"stdbuf": "ioe", "flock": "wE", "strace": "oepsPEu", "ltrace": "oepsu", "runuser": "ugG",
	"systemd-run": "puEtGP", "timeout": "ks", "exec": "a",
}

var wrapperLongArgs = map[string]*regexp.Regexp{
	"systemd-run": regexp.MustCompile(`^--(unit|property|setenv|working-directory|uid|gid|nice|description|slice|service-type|on-active|on-boot|on-calendar|timer-property)$`),
	"unshare":     regexp.MustCompile(`^--(setuid|setgid|map-user|map-group|wd|kill-child)$`),
	"firejail":    regexp.MustCompile(`^--(profile|name|hostname|netns|private|whitelist|blacklist)$`),
	"setpriv":     regexp.MustCompile(`^--(reuid|regid|groups|ambient-caps|bounding-set|selinux-label|apparmor-profile)$`),
	"timeout":     regexp.MustCompile(`^--(signal|kill-after)$`),
	"env":         regexp.MustCompile(`^--(unset|chdir)$`),
	"flock":       regexp.MustCompile(`^--(timeout|wait|conflict-exit-code)$`),
	"runuser":     regexp.MustCompile(`^--(user|group|supp-group|shell|whitelist-environment)$`),
	"sudo":        regexp.MustCompile(`^--(user|group|close-from|chroot|chdir|prompt|role|type|host|other-user|command-timeout)$`),
}

var wrappers = map[string]bool{
	"sudo": true, "doas": true, "env": true, "timeout": true, "nice": true, "ionice": true, "chrt": true,
	"taskset": true, "nohup": true, "stdbuf": true, "time": true, "command": true, "exec": true, "builtin": true,
	"flock": true, "systemd-run": true, "unshare": true, "firejail": true, "pkexec": true, "caffeinate": true,
	"setsid": true, "strace": true, "ltrace": true, "setpriv": true, "runuser": true,
}

var (
	assignRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	cpuListRe    = regexp.MustCompile(`^[0-9a-fx,-]+$`)
	xargsLong    = regexp.MustCompile(`^--(arg-file|delimiter|max-args|max-procs|max-chars|process-slot-var)$`)
	parallelLong = regexp.MustCompile(`^--(arg-file|delimiter|max-args|max-procs|max-chars|max-lines|replace|jobs|sshlogin|colsep)$`)
	watchLong    = regexp.MustCompile(`^--(interval|differences|equexit)$`)
	nsenterLong  = regexp.MustCompile(`^--(target|setuid|setgid|root|wd)$`)
	machineLong  = regexp.MustCompile(`^--(uid|setenv)$`)
	chrootLong   = regexp.MustCompile(`^--(userspec|groups)$`)
)

const (
	xargsArgs    = "adEILnPs"
	parallelArgs = "adEIjLnPsS"
)

var detachers = map[string]bool{"nohup": true, "setsid": true, "disown": true}

// Unwrap strips wrapper programs and leading assignments so that argv[0] is the
// program that actually does the work.
func Unwrap(argv []string) []string {
	rest, _ := unwrap(argv)
	return rest
}

func unwrap(argv []string) (rest, heads []string) {
	for guard := 0; guard < 16 && len(argv) > 0; guard++ {
		if assignRe.MatchString(argv[0]) {
			argv = argv[1:]
			continue
		}
		if _, ok := carrier(argv); ok {
			return argv, heads
		}
		b := base(argv[0])
		next, ok := strip(b, argv)
		if !ok {
			return argv, heads
		}
		heads = append(heads, b)
		argv = next
	}
	return argv, heads
}

func detachedBy(heads, argv []string) bool {
	for _, h := range heads {
		if detachers[h] {
			return true
		}
	}
	return len(argv) > 0 && detachers[base(argv[0])]
}

func strip(b string, argv []string) ([]string, bool) {
	switch b {
	case "ip":
		if len(argv) >= 4 && argv[1] == "netns" && argv[2] == "exec" {
			return argv[4:], true
		}
		return nil, false
	case "bwrap":
		return bwrapCommand(argv[1:])
	case "nix":
		if len(argv) < 2 || (argv[1] != "shell" && argv[1] != "develop" && argv[1] != "run") {
			return nil, false
		}
		rest := afterFlag(argv[2:], "-c", "--command")
		if rest == nil && argv[1] == "run" && len(argv) >= 3 {
			if after := afterDashDash(argv[3:]); after != nil {
				rest = append([]string{flakeProgram(argv[2])}, after...)
			}
		}
		return rest, rest != nil
	case "xargs":
		return skipOptions(argv[1:], xargsArgs, xargsLong), true
	case "parallel":
		return skipOptions(argv[1:], parallelArgs, parallelLong), true
	case "watch":
		return skipOptions(argv[1:], "nq", watchLong), true
	case "command":
		for _, w := range argv[1:] {
			if !strings.HasPrefix(w, "-") || w == "--" {
				break
			}
			if strings.ContainsAny(w[1:], "vV") {
				return nil, false
			}
		}
		return skipOptions(argv[1:], "", nil), true
	}
	if !wrappers[b] {
		return nil, false
	}
	argv = skipOptions(argv[1:], wrapperArgs[b], wrapperLongArgs[b])
	switch b {
	case "timeout", "flock":
		if len(argv) > 0 {
			argv = argv[1:]
		}
	case "chrt", "taskset":
		if len(argv) > 0 && cpuListRe.MatchString(argv[0]) {
			argv = argv[1:]
		}
	}
	return argv, true
}

// flakeProgram guesses the program a flake reference runs: the last attribute
// after '#', or the last path element.
func flakeProgram(ref string) string {
	if i := strings.LastIndexByte(ref, '#'); i >= 0 {
		ref = ref[i+1:]
	}
	if i := strings.LastIndexByte(ref, '.'); i >= 0 && !strings.Contains(ref[i+1:], "/") {
		ref = ref[i+1:]
	}
	return base(ref)
}

var bwrapArity = map[string]int{
	"--help": 0, "--version": 0, "--unshare-all": 0, "--share-net": 0, "--unshare-user": 0, "--unshare-user-try": 0,
	"--unshare-ipc": 0, "--unshare-pid": 0, "--unshare-net": 0, "--unshare-uts": 0, "--unshare-cgroup": 0,
	"--unshare-cgroup-try": 0, "--clearenv": 0, "--new-session": 0, "--die-with-parent": 0, "--as-pid-1": 0,
	"--disable-userns": 0, "--assert-userns-disabled": 0,
	"--args": 1, "--argv0": 1, "--userns": 1, "--userns2": 1, "--pidns": 1, "--uid": 1, "--gid": 1, "--hostname": 1,
	"--chdir": 1, "--unsetenv": 1, "--lock-file": 1, "--sync-fd": 1, "--remount-ro": 1, "--exec-label": 1,
	"--file-label": 1, "--proc": 1, "--dev": 1, "--tmpfs": 1, "--mqueue": 1, "--dir": 1, "--seccomp": 1,
	"--add-seccomp-fd": 1, "--block-fd": 1, "--userns-block-fd": 1, "--info-fd": 1, "--json-status-fd": 1,
	"--cap-add": 1, "--cap-drop": 1, "--perms": 1, "--size": 1, "--overlay-src": 1, "--tmp-overlay": 1, "--ro-overlay": 1,
	"--setenv": 2, "--bind": 2, "--bind-try": 2, "--dev-bind": 2, "--dev-bind-try": 2, "--ro-bind": 2, "--ro-bind-try": 2,
	"--bind-fd": 2, "--ro-bind-fd": 2, "--file": 2, "--bind-data": 2, "--ro-bind-data": 2, "--symlink": 2, "--chmod": 2,
	"--overlay": 3,
}

func bwrapCommand(argv []string) ([]string, bool) {
	for len(argv) > 0 && strings.HasPrefix(argv[0], "--") {
		if argv[0] == "--" {
			return argv[1:], true
		}
		n, ok := bwrapArity[argv[0]]
		if !ok || len(argv) <= n {
			return nil, false
		}
		argv = argv[n+1:]
	}
	return argv, true
}

// carrier returns the shell string a wrapper runs instead of a program of its
// own: env -S, flock -c, watch without -x, sudo -s or -i, runuser -c.
func carrier(argv []string) (string, bool) {
	if len(argv) < 2 {
		return "", false
	}
	switch base(argv[0]) {
	case "env":
		for i := 1; i < len(argv) && strings.HasPrefix(argv[i], "-"); i++ {
			w := argv[i]
			switch {
			case w == "--":
				return "", false
			case w == "-S" || w == "--split-string":
				if i+1 < len(argv) {
					return strings.Join(argv[i+1:], " "), true
				}
				return "", false
			case strings.HasPrefix(w, "--split-string="):
				return strings.Join(append([]string{strings.TrimPrefix(w, "--split-string=")}, argv[i+1:]...), " "), true
			case !strings.HasPrefix(w, "--") && strings.Contains(w, "S"):
				k := strings.IndexByte(w, 'S')
				if k == len(w)-1 && i+1 < len(argv) {
					return strings.Join(argv[i+1:], " "), true
				}
				return strings.Join(append([]string{w[k+1:]}, argv[i+1:]...), " "), true
			case w == "-u" || w == "-C" || w == "--unset" || w == "--chdir":
				i++
			}
		}
	case "flock", "runuser":
		for i := 1; i+1 < len(argv); i++ {
			if argv[i] == "--" {
				return "", false
			}
			if argv[i] == "-c" || argv[i] == "--command" {
				return argv[i+1], true
			}
		}
		for _, w := range argv[1:] {
			if strings.HasPrefix(w, "--command=") {
				return strings.TrimPrefix(w, "--command="), true
			}
		}
	case "watch":
		for _, w := range argv[1:] {
			if w == "-x" || w == "--exec" || (strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.Contains(w, "x")) {
				return "", false
			}
		}
		if rest := skipOptions(argv[1:], "nq", watchLong); len(rest) > 0 {
			return strings.Join(rest, " "), true
		}
	case "sudo":
		shell := false
		rest := argv[1:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			w := rest[0]
			if w == "--" {
				rest = rest[1:]
				break
			}
			short := !strings.HasPrefix(w, "--")
			if w == "--shell" || w == "--login" || (short && strings.ContainsAny(w[1:], "si")) {
				shell = true
			}
			takes := (short && len(w) == 2 && strings.ContainsRune(wrapperArgs["sudo"], rune(w[1]))) || wrapperLongArgs["sudo"].MatchString(w)
			if takes && len(rest) > 1 {
				rest = rest[2:]
			} else {
				rest = rest[1:]
			}
		}
		if shell && len(rest) > 0 {
			return strings.Join(rest, " "), true
		}
	}
	return "", false
}

func skipOptions(argv []string, shortArgs string, longArgs *regexp.Regexp) []string {
	for len(argv) > 0 && strings.HasPrefix(argv[0], "-") {
		w := argv[0]
		if w == "--" {
			return argv[1:]
		}
		if strings.Contains(w, "=") {
			argv = argv[1:]
			continue
		}
		takesArg := false
		if !strings.HasPrefix(w, "--") && len(w) == 2 && strings.ContainsRune(shortArgs, rune(w[1])) {
			takesArg = true
		}
		if longArgs != nil && longArgs.MatchString(w) {
			takesArg = true
		}
		if takesArg && len(argv) > 1 {
			argv = argv[2:]
		} else {
			argv = argv[1:]
		}
	}
	return argv
}

func afterFlag(argv []string, flags ...string) []string {
	for i, w := range argv {
		for _, f := range flags {
			if w == f {
				return argv[i+1:]
			}
		}
	}
	return nil
}

func afterDashDash(argv []string) []string {
	for i, w := range argv {
		if w == "--" {
			return argv[i+1:]
		}
	}
	return nil
}

type Payload struct {
	Text string
	Kind Kind
}

// rawTail renders a trailing slice of the command's arguments the way a remote
// shell will receive them: one word gives its unquoted value, several words
// keep their original quoting because the remote side parses them again.
func rawTail(cmd Command, tail []string) string {
	if len(tail) == 1 {
		return tail[0]
	}
	off := len(cmd.Argv) - len(tail)
	if off < 0 || off > len(cmd.Words) || len(cmd.Words) != len(cmd.Argv) {
		return strings.Join(tail, " ")
	}
	return strings.Join(cmd.Words[off:], " ")
}

// Payloads returns the command strings that cmd hands to another interpreter.
func Payloads(cmd Command) []Payload {
	argv := Unwrap(cmd.Argv)
	if len(argv) == 0 {
		return nil
	}
	head := base(argv[0])
	kind := cmd.Kind
	var out []Payload
	emit := func(text string, k Kind) {
		if strings.TrimSpace(text) != "" {
			out = append(out, Payload{Text: text, Kind: k})
		}
	}
	piped := func(words []string, k Kind) {
		if cmd.Stdin != "" && len(words) > 0 && shells[base(words[0])] && readsScript(words[1:]) {
			emit(cmd.Stdin, k)
		}
	}

	if !remoteHeads[head] {
		for _, s := range shellDashC(argv) {
			emit(s, kind)
		}
	}
	if s, ok := carrier(argv); ok {
		emit(s, kind)
		return out
	}

	switch {
	case head == "ssh" || head == "autossh":
		rest := sshRemote(argv[1:])
		if len(rest) == 0 {
			emit(cmd.Heredoc+cmd.Stdin, Remote)
			return out
		}
		emit(rawTail(cmd, rest), Remote)
		if cmd.Heredoc != "" && shells[firstWord(rest[0])] {
			emit(cmd.Heredoc, Remote)
		}
		piped(strings.Fields(rawTail(cmd, rest)), Remote)
	case head == "adb":
		rest := argv[1:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			if rest[0] == "-s" || rest[0] == "-t" || rest[0] == "-H" || rest[0] == "-P" {
				rest = rest[1:]
			}
			if len(rest) > 0 {
				rest = rest[1:]
			}
		}
		if len(rest) > 1 && rest[0] == "shell" {
			emit(rawTail(cmd, rest[1:]), Remote)
		}
	case shells[head]:
		if cmd.Heredoc != "" && !hasDashC(argv[1:]) {
			emit(cmd.Heredoc, kind)
		}
		piped(argv, kind)
	case head == "busybox" && len(argv) > 1 && shells[argv[1]]:
		if cmd.Heredoc != "" && !hasDashC(argv[2:]) {
			emit(cmd.Heredoc, kind)
		}
		piped(argv[1:], kind)
	case head == "su":
		if s := afterFlag(argv[1:], "-c", "--command"); len(s) > 0 {
			emit(s[0], kind)
		} else {
			emit(cmd.Heredoc+cmd.Stdin, kind)
		}
		for _, w := range argv[1:] {
			if strings.HasPrefix(w, "--command=") {
				emit(strings.TrimPrefix(w, "--command="), kind)
			}
		}
	case head == "eval":
		emit(strings.Join(argv[1:], " "), kind)
	case head == "nix-shell":
		if s := afterFlag(argv[1:], "--run", "--command"); len(s) > 0 {
			emit(s[0], kind)
		}
	case head == "docker" || head == "podman" || head == "nerdctl":
		rest := argv[1:]
		if len(rest) > 0 && rest[0] == "container" {
			rest = rest[1:]
		}
		if len(rest) == 0 || (rest[0] != "exec" && rest[0] != "run") {
			return out
		}
		entrypoint := ""
		for i, w := range rest {
			if w == "--entrypoint" && i+1 < len(rest) {
				entrypoint = rest[i+1]
			}
			if strings.HasPrefix(w, "--entrypoint=") {
				entrypoint = strings.TrimPrefix(w, "--entrypoint=")
			}
		}
		rest = skipOptions(rest[1:], "uwevphlm", dockerLongArgs)
		if len(rest) > 1 {
			payload := rawTail(cmd, rest[1:])
			if entrypoint != "" {
				payload = entrypoint + " " + rawTail(cmd, rest[1:])
			}
			emit(payload, Remote)
			if cmd.Heredoc != "" && (shells[firstWord(payload)]) {
				emit(cmd.Heredoc, Remote)
			}
			piped(strings.Fields(payload), Remote)
		}
	case head == "kubectl" || head == "oc" || head == "incus" || head == "lxc" || head == "distrobox" || head == "toolbox" || head == "nsenter":
		rest := afterDashDash(argv[1:])
		if rest == nil && head == "nsenter" {
			rest = skipOptions(argv[1:], "tSGrw", nsenterLong)
		}
		if len(rest) > 0 {
			emit(rawTail(cmd, rest), Remote)
			if cmd.Heredoc != "" && shells[base(rest[0])] {
				emit(cmd.Heredoc, Remote)
			}
			piped(rest, Remote)
		}
	case head == "machinectl" && len(argv) > 1 && argv[1] == "shell":
		rest := skipOptions(argv[2:], "", machineLong)
		if len(rest) > 1 {
			emit(rawTail(cmd, rest[1:]), Remote)
		}
	case head == "chroot":
		rest := skipOptions(argv[1:], "", chrootLong)
		if len(rest) > 1 {
			emit(rawTail(cmd, rest[1:]), Remote)
			if cmd.Heredoc != "" && shells[base(rest[1])] {
				emit(cmd.Heredoc, Remote)
			}
			piped(rest[1:], Remote)
		}
	case head == "script":
		if s := afterFlag(argv[1:], "-c", "--command"); len(s) > 0 {
			emit(s[0], kind)
		}
	case strings.HasPrefix(head, "python"):
		if s := afterFlag(argv[1:], "-c"); len(s) > 0 {
			for _, p := range pythonShellStrings(s[0]) {
				emit(p, kind)
			}
		} else if cmd.Heredoc != "" {
			for _, p := range pythonShellStrings(cmd.Heredoc) {
				emit(p, kind)
			}
		}
	}
	return out
}

var dockerLongArgs = regexp.MustCompile(`^--(user|workdir|env|env-file|name|volume|publish|entrypoint|network|memory|hostname|label|mount|add-host|cpus|gpus|platform|pull|restart|tmpfs|ulimit|device|cap-add|cap-drop|security-opt|log-driver|log-opt|dns|ip|pid|ipc|uts|userns|cgroupns|shm-size|stop-signal|stop-timeout|health-cmd|detach-keys)$`)

const sshArgOpts = "BbcDEeFIiJLlmOoPpQRSWw"

// sshRemote returns the words ssh will join and send to the remote shell.
func sshRemote(argv []string) []string {
	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		w := argv[i]
		if w == "--" {
			i++
			break
		}
		if len(w) == 2 && strings.ContainsRune(sshArgOpts, rune(w[1])) {
			i += 2
			continue
		}
		i++
	}
	i++ // destination
	if i >= len(argv) {
		return nil
	}
	return argv[i:]
}

func hasDashC(argv []string) bool {
	for _, w := range argv {
		if w == "--" {
			return false
		}
		if strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.Contains(w, "c") {
			return true
		}
		if w == "--command" || strings.HasPrefix(w, "--command=") {
			return true
		}
		if !strings.HasPrefix(w, "-") {
			return false
		}
	}
	return false
}

func readsScript(args []string) bool {
	for i := 0; i < len(args); i++ {
		w := args[i]
		switch {
		case w == "--" || w == "-":
			return i+1 >= len(args)
		case w == "--command" || strings.HasPrefix(w, "--command="):
			return false
		case w == "--rcfile" || w == "--init-file":
			i++
		case strings.HasPrefix(w, "--"):
		case strings.HasPrefix(w, "-") || strings.HasPrefix(w, "+"):
			if strings.Contains(w, "c") {
				return false
			}
			if strings.Contains(w, "s") {
				return true
			}
			if last := w[len(w)-1]; last == 'o' || last == 'O' {
				i++
			}
		default:
			return false
		}
	}
	return true
}

// shellDashC finds every `sh -c STRING` anywhere in argv, so that `fd -x sh -c`,
// `xargs sh -c`, `nix shell ... -c bash -c` and friends all surface their payload.
func shellDashC(argv []string) []string {
	var out []string
	for i := 0; i+1 < len(argv); i++ {
		if !shells[base(argv[i])] {
			continue
		}
		j := i + 1
		for j < len(argv) && (strings.HasPrefix(argv[j], "-") || strings.HasPrefix(argv[j], "+")) {
			w := argv[j]
			if w == "--" {
				break
			}
			if strings.HasPrefix(w, "--command=") {
				out = append(out, strings.TrimPrefix(w, "--command="))
				break
			}
			if w == "--command" || (!strings.HasPrefix(w, "--") && strings.Contains(w, "c")) {
				if j+1 < len(argv) {
					out = append(out, argv[j+1])
				}
				break
			}
			if shellOptArgs[w] {
				j += 2
				continue
			}
			j++
		}
	}
	return out
}

var shellOptArgs = map[string]bool{"-o": true, "+o": true, "-O": true, "+O": true, "--rcfile": true, "--init-file": true}

func IsInterpreter(head string) bool {
	return strings.HasPrefix(head, "python") || head == "node" || head == "bun" || head == "deno"
}

var pyCall = regexp.MustCompile(`(?:os\.system|subprocess\.[A-Za-z_]+)\(\s*[rbfu]?(?:"""|'''|"|')`)

// pythonShellStrings pulls the string literals handed to os.system and
// subprocess.* so that a shell command hidden in Python is still inspected.
func pythonShellStrings(code string) []string {
	var out []string
	for _, loc := range pyCall.FindAllStringIndex(code, -1) {
		rest := code[loc[1]:]
		quote := code[loc[1]-1 : loc[1]]
		if strings.HasSuffix(code[:loc[1]], `"""`) || strings.HasSuffix(code[:loc[1]], `'''`) {
			quote = code[loc[1]-3 : loc[1]]
		}
		var b strings.Builder
		for i := 0; i < len(rest); i++ {
			if strings.HasPrefix(rest[i:], quote) {
				break
			}
			if rest[i] == '\\' && i+1 < len(rest) {
				b.WriteByte(rest[i+1])
				i++
				continue
			}
			b.WriteByte(rest[i])
		}
		out = append(out, b.String())
	}
	return out
}

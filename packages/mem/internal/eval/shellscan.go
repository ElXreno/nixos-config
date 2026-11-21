package eval

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Effect struct {
	Path  string
	Write bool
	Op    string
}

type ShellScan struct {
	Effects  []Effect
	Commands []string
}

type shWord struct {
	s    string
	glob bool
	dyn  bool
	loop bool
}

type shRedir struct {
	op     string
	target shWord
}

type shCmd struct {
	words  []shWord
	redirs []shRedir
}

type heredoc struct {
	delim string
	strip bool
}

type shScanner struct {
	home   string
	vars   map[string]string
	cwd    string
	loops  map[string][]string
	accept func(string) bool
	res    ShellScan
	depth  int
}

const maxShellDepth = 6

func ScanShell(cmd, home string, accept func(string) bool) ShellScan {
	sc := &shScanner{home: home, vars: map[string]string{}, loops: map[string][]string{}, accept: accept}
	if home != "" {
		sc.vars["HOME"] = home
	}
	sc.parse(cmd)
	return sc.res
}

var (
	readCmds = map[string]bool{
		"cat": true, "tac": true, "nl": true, "head": true, "tail": true, "less": true, "more": true, "bat": true, "batcat": true,
		"sed": true, "awk": true, "gawk": true, "mawk": true, "nawk": true, "rg": true, "grep": true, "egrep": true, "fgrep": true,
		"ag": true, "ack": true,
	}
	shellCmds    = map[string]bool{"bash": true, "sh": true, "dash": true, "zsh": true, "fish": true, "ksh": true}
	reservedHead = map[string]bool{
		"if": true, "then": true, "else": true, "elif": true, "fi": true, "while": true, "until": true, "do": true, "done": true,
		"{": true, "}": true, "!": true, "esac": true,
	}
	skippedHead     = map[string]bool{"for": true, "case": true, "select": true, "function": true}
	interpreterCmds = map[string]bool{
		"python": true, "python3": true, "perl": true, "ruby": true, "node": true, "nodejs": true, "lua": true, "php": true, "go": true,
		"bun": true, "deno": true,
	}
	sedInPlaceRe  = regexp.MustCompile(`^-[a-zA-Z]*i`)
	perlInPlaceRe = regexp.MustCompile(`^-[a-zA-Z]*i`)
	assignRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	varNameRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
)

func isInterpreter(name string) bool {
	if interpreterCmds[name] {
		return true
	}
	for _, p := range []string{"python3.", "perl5."} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func (sc *shScanner) parse(src string) {
	if sc.depth > maxShellDepth {
		return
	}
	sc.depth++
	defer func() { sc.depth-- }()
	var (
		cur      shCmd
		sb       strings.Builder
		started  bool
		glob     bool
		dyn      bool
		loop     bool
		quoted   bool
		pending  string
		heredocs []heredoc
	)
	flushWord := func() {
		if !started {
			return
		}
		w := shWord{s: sb.String(), glob: glob, dyn: dyn, loop: loop}
		sb.Reset()
		started, glob, dyn, loop = false, false, false, false
		quoted = false
		if pending != "" {
			op := pending
			pending = ""
			switch op {
			case "<<", "<<-":
				heredocs = append(heredocs, heredoc{delim: w.s, strip: op == "<<-"})
			case "<<<":
			default:
				cur.redirs = append(cur.redirs, shRedir{op: op, target: w})
			}
			return
		}
		cur.words = append(cur.words, w)
	}
	flushCmd := func() {
		flushWord()
		if len(cur.words) > 0 || len(cur.redirs) > 0 {
			sc.finish(cur)
		}
		cur = shCmd{}
		pending = ""
	}
	setRedir := func(op string) {
		if started && !quoted {
			digits := true
			for _, r := range sb.String() {
				if r < '0' || r > '9' {
					digits = false
				}
			}
			if digits {
				sb.Reset()
				started, glob, dyn, loop = false, false, false, false
			}
		}
		flushWord()
		pending = op
	}
	n := len(src)
	i := 0
	for i < n {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			flushWord()
			i++
		case c == '\n':
			flushCmd()
			i++
			for _, h := range heredocs {
				i = skipHeredoc(src, i, h)
			}
			heredocs = nil
		case c == '#' && !started:
			for i < n && src[i] != '\n' {
				i++
			}
		case c == ';' || c == '(' || c == ')':
			flushCmd()
			i++
		case c == '|':
			flushCmd()
			i++
			if i < n && (src[i] == '|' || src[i] == '&') {
				i++
			}
		case c == '&':
			if i+1 < n && src[i+1] == '>' {
				op := "&>"
				i += 2
				if i < n && src[i] == '>' {
					op = "&>>"
					i++
				}
				setRedir(op)
				continue
			}
			flushCmd()
			i++
			if i < n && src[i] == '&' {
				i++
			}
		case c == '<':
			switch {
			case strings.HasPrefix(src[i:], "<<<"):
				setRedir("<<<")
				i += 3
			case strings.HasPrefix(src[i:], "<<-"):
				setRedir("<<-")
				i += 3
			case strings.HasPrefix(src[i:], "<<"):
				setRedir("<<")
				i += 2
			case strings.HasPrefix(src[i:], "<("):
				end := matchParen(src, i+1)
				sc.parse(src[i+2 : end])
				started, dyn = true, true
				i = end + 1
			case strings.HasPrefix(src[i:], "<&"):
				setRedir("<&")
				i += 2
			case strings.HasPrefix(src[i:], "<>"):
				setRedir("<>")
				i += 2
			default:
				setRedir("<")
				i++
			}
		case c == '>':
			switch {
			case strings.HasPrefix(src[i:], ">("):
				end := matchParen(src, i+1)
				sc.parse(src[i+2 : end])
				started, dyn = true, true
				i = end + 1
			case strings.HasPrefix(src[i:], ">>"):
				setRedir(">>")
				i += 2
			case strings.HasPrefix(src[i:], ">|"):
				setRedir(">|")
				i += 2
			case strings.HasPrefix(src[i:], ">&"):
				setRedir(">&")
				i += 2
			default:
				setRedir(">")
				i++
			}
		case c == '\\':
			if i+1 < n {
				if src[i+1] == '\n' {
					i += 2
					continue
				}
				sb.WriteByte(src[i+1])
				started, quoted = true, true
				i += 2
			} else {
				i++
			}
		case c == '\'':
			end := strings.IndexByte(src[i+1:], '\'')
			started, quoted = true, true
			if end < 0 {
				sb.WriteString(src[i+1:])
				i = n
			} else {
				sb.WriteString(src[i+1 : i+1+end])
				i += end + 2
			}
		case c == '"':
			started, quoted = true, true
			i++
			for i < n && src[i] != '"' {
				switch src[i] {
				case '\\':
					if i+1 < n {
						switch src[i+1] {
						case '$', '`', '"', '\\':
							sb.WriteByte(src[i+1])
							i += 2
						case '\n':
							i += 2
						default:
							sb.WriteByte('\\')
							i++
						}
					} else {
						i++
					}
				case '$':
					i = sc.expandDollar(src, i, &sb, &dyn, &loop)
				case '`':
					i = sc.expandBacktick(src, i, &dyn)
				default:
					sb.WriteByte(src[i])
					i++
				}
			}
			i++
		case c == '$':
			started = true
			i = sc.expandDollar(src, i, &sb, &dyn, &loop)
		case c == '`':
			started = true
			i = sc.expandBacktick(src, i, &dyn)
		case c == '~' && (!started || assignRe.FindString(sb.String()) == sb.String() && !quoted):
			started = true
			j := i + 1
			if j == n || src[j] == '/' || src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == ';' || src[j] == '|' || src[j] == '&' || src[j] == ')' {
				if sc.home == "" {
					dyn = true
				} else {
					sb.WriteString(sc.home)
				}
			} else {
				sb.WriteByte('~')
				dyn = true
			}
			i++
		case c == '*' || c == '?' || c == '[':
			started, glob = true, true
			sb.WriteByte(c)
			i++
		case c == '{':
			started = true
			sb.WriteByte(c)
			if sb.Len() > 1 {
				glob = true
			} else if i+1 < n && src[i+1] != ' ' && src[i+1] != '\t' && src[i+1] != '\n' {
				glob = true
			}
			i++
		default:
			started = true
			sb.WriteByte(c)
			i++
		}
	}
	flushCmd()
}

func skipHeredoc(src string, i int, h heredoc) int {
	n := len(src)
	for i < n {
		end := strings.IndexByte(src[i:], '\n')
		line := src[i:]
		next := n
		if end >= 0 {
			line = src[i : i+end]
			next = i + end + 1
		}
		if h.strip {
			line = strings.TrimLeft(line, "\t")
		}
		i = next
		if strings.TrimRight(line, "\r") == h.delim {
			return i
		}
	}
	return n
}

func matchParen(src string, open int) int {
	depth := 0
	n := len(src)
	for i := open; i < n; i++ {
		switch src[i] {
		case '\\':
			i++
		case '\'':
			if end := strings.IndexByte(src[i+1:], '\''); end >= 0 {
				i += end + 1
			}
		case '"':
			for i++; i < n && src[i] != '"'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return n
}

func (sc *shScanner) expandBacktick(src string, i int, dyn *bool) int {
	n := len(src)
	j := i + 1
	for j < n && src[j] != '`' {
		if src[j] == '\\' {
			j++
		}
		j++
	}
	if j > n {
		j = n
	}
	inner := src[i+1 : j]
	sc.parse(strings.ReplaceAll(inner, "\\`", "`"))
	*dyn = true
	return min(j+1, n)
}

func (sc *shScanner) expandDollar(src string, i int, sb *strings.Builder, dyn, loop *bool) int {
	n := len(src)
	if i+1 >= n {
		sb.WriteByte('$')
		return i + 1
	}
	switch src[i+1] {
	case '(':
		end := matchParen(src, i+1)
		if strings.HasPrefix(src[i:], "$((") {
			*dyn = true
			return min(end+1, n)
		}
		sc.parse(src[i+2 : min(end, n)])
		*dyn = true
		return min(end+1, n)
	case '{':
		end := strings.IndexByte(src[i:], '}')
		if end < 0 {
			*dyn = true
			return n
		}
		name := src[i+2 : i+end]
		if _, ok := sc.loops[name]; ok && varNameRe.FindString(name) == name {
			sb.WriteString(loopMark(name))
			*loop = true
		} else if v, ok := sc.vars[name]; ok && varNameRe.FindString(name) == name {
			sb.WriteString(v)
		} else {
			*dyn = true
		}
		return i + end + 1
	case '\'':
		end := strings.IndexByte(src[i+2:], '\'')
		if end < 0 {
			*dyn = true
			return n
		}
		body := src[i+2 : i+2+end]
		if strings.Contains(body, "\\") {
			*dyn = true
		}
		sb.WriteString(body)
		return i + end + 3
	default:
		name := varNameRe.FindString(src[i+1:])
		if name == "" {
			*dyn = true
			return i + 2
		}
		if _, ok := sc.loops[name]; ok {
			sb.WriteString(loopMark(name))
			*loop = true
		} else if v, ok := sc.vars[name]; ok {
			sb.WriteString(v)
		} else {
			*dyn = true
		}
		return i + 1 + len(name)
	}
}

const maxLoopWords = 256

var loopMarkRe = regexp.MustCompile("\x01([A-Za-z_][A-Za-z0-9_]*)\x02")

func loopMark(name string) string { return "\x01" + name + "\x02" }

func (sc *shScanner) values(w shWord) []string {
	if w.dyn || w.glob || w.s == "" {
		return nil
	}
	if !w.loop {
		return []string{w.s}
	}
	out := []string{w.s}
	for _, m := range loopMarkRe.FindAllStringSubmatch(w.s, -1) {
		vals, ok := sc.loops[m[1]]
		if !ok {
			return nil
		}
		var next []string
		for _, s := range out {
			for _, v := range vals {
				next = append(next, strings.ReplaceAll(s, m[0], v))
			}
		}
		if len(next) > maxLoopWords {
			return nil
		}
		out = next
	}
	return out
}

func (sc *shScanner) place(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if !filepath.IsAbs(s) {
		if sc.cwd == "" {
			return "", false
		}
		s = filepath.Join(sc.cwd, s)
	}
	return filepath.Clean(s), true
}

func (sc *shScanner) resolve(w shWord) (string, bool) {
	if w.loop {
		return "", false
	}
	vals := sc.values(w)
	if len(vals) != 1 {
		return "", false
	}
	return sc.place(vals[0])
}

func (sc *shScanner) add(w shWord, write bool, op string) {
	for _, v := range sc.values(w) {
		p, ok := sc.place(v)
		if !ok || sc.accept != nil && !sc.accept(p) {
			continue
		}
		sc.res.Effects = append(sc.res.Effects, Effect{Path: p, Write: write, Op: op})
	}
}

func operandsOf(words []shWord) []shWord {
	var out []shWord
	rest := false
	for _, w := range words {
		if !rest && w.s == "--" && !w.dyn {
			rest = true
			continue
		}
		if !rest && strings.HasPrefix(w.s, "-") && len(w.s) > 1 {
			continue
		}
		out = append(out, w)
	}
	return out
}

func hasFlag(words []shWord, re *regexp.Regexp) bool {
	for _, w := range words {
		if strings.HasPrefix(w.s, "--in-place") || re.MatchString(w.s) && !strings.HasPrefix(w.s, "--") {
			return true
		}
	}
	return false
}

func (sc *shScanner) finish(c shCmd) {
	for _, r := range c.redirs {
		switch r.op {
		case ">", ">>", ">|", "&>", "&>>":
			sc.add(r.target, true, "redirect")
		case ">&":
			if r.target.s != "-" && !isDigits(r.target.s) {
				sc.add(r.target, true, "redirect")
			}
		case "<", "<>":
			sc.add(r.target, false, "redirect-in")
		}
	}
	sc.command(c.words)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func baseName(w shWord) string {
	if w.dyn {
		return ""
	}
	return filepath.Base(w.s)
}

func (sc *shScanner) command(words []shWord) {
	for len(words) > 0 && reservedHead[baseName(words[0])] {
		words = words[1:]
	}
	for len(words) > 0 && assignRe.MatchString(words[0].s) && !strings.HasPrefix(words[0].s, "=") {
		eq := strings.IndexByte(words[0].s, '=')
		delete(sc.loops, words[0].s[:eq])
		if !words[0].dyn && !words[0].glob && !words[0].loop {
			sc.vars[words[0].s[:eq]] = words[0].s[eq+1:]
		} else {
			delete(sc.vars, words[0].s[:eq])
		}
		words = words[1:]
	}
	for len(words) > 0 {
		name := baseName(words[0])
		switch {
		case reservedHead[name]:
			words = words[1:]
			continue
		case name == "sudo" || name == "doas":
			words = skipOptions(words[1:], map[string]bool{"-u": true, "-g": true, "-C": true, "-h": true, "-p": true, "-r": true, "-t": true, "-U": true, "-D": true})
			continue
		case name == "env":
			rest := words[1:]
			for len(rest) > 0 {
				switch {
				case rest[0].s == "-u" || rest[0].s == "-C" || rest[0].s == "-S":
					rest = rest[min(2, len(rest)):]
				case strings.HasPrefix(rest[0].s, "-") || assignRe.MatchString(rest[0].s):
					rest = rest[1:]
				default:
					goto envDone
				}
			}
		envDone:
			words = rest
			continue
		case name == "time" || name == "nohup" || name == "command" || name == "exec" || name == "builtin" || name == "setsid":
			words = skipOptions(words[1:], nil)
			continue
		case name == "nice" || name == "ionice" || name == "stdbuf":
			words = skipOptions(words[1:], map[string]bool{"-n": true, "-c": true, "-i": true, "-o": true, "-e": true})
			continue
		case name == "timeout":
			rest := skipOptions(words[1:], map[string]bool{"-s": true, "-k": true})
			words = rest[min(1, len(rest)):]
			continue
		}
		break
	}
	if len(words) == 0 {
		return
	}
	name := baseName(words[0])
	if name == "for" {
		sc.forHeader(words[1:])
		return
	}
	if name == "" || skippedHead[name] {
		return
	}
	sc.res.Commands = append(sc.res.Commands, name)
	args := words[1:]
	switch {
	case name == "cd" || name == "pushd":
		ops := operandsOf(args)
		switch {
		case len(ops) == 0 && name == "cd" && sc.home != "":
			sc.cwd = sc.home
		case len(ops) == 1:
			if p, ok := sc.resolve(ops[0]); ok {
				sc.cwd = p
			} else {
				sc.cwd = ""
			}
		default:
			sc.cwd = ""
		}
	case shellCmds[name]:
		for i, a := range args {
			if a.s == "-c" || strings.HasPrefix(a.s, "-") && !strings.HasPrefix(a.s, "--") && strings.Contains(a.s, "c") && len(a.s) <= 4 {
				if i+1 < len(args) && !args[i+1].dyn {
					sc.parse(args[i+1].s)
				}
				break
			}
		}
	case name == "nix":
		for i, a := range args {
			if a.s == "-c" || a.s == "--command" {
				sc.command(args[i+1:])
				break
			}
		}
	case name == "sed":
		sc.fileArgs(args, hasFlag(args, sedInPlaceRe), "sed-i", name)
	case name == "awk" || name == "gawk" || name == "mawk" || name == "nawk":
		inplace := false
		for _, a := range args {
			if a.s == "inplace" {
				inplace = true
			}
		}
		sc.fileArgs(args, inplace, "awk-inplace", name)
	case readCmds[name]:
		sc.fileArgs(args, false, "", name)
	case name == "perl" || name == "ruby":
		if hasFlag(args, perlInPlaceRe) {
			sc.fileArgs(args, true, name+"-i", name)
		}
	case name == "tee" || name == "sponge" || name == "touch" || name == "truncate" || name == "unlink":
		op := name
		for _, o := range operandsOf(args) {
			sc.add(o, true, op)
		}
	case name == "rm":
		for _, o := range operandsOf(args) {
			sc.add(o, true, "rm")
		}
	case name == "mv" || name == "cp" || name == "install" || name == "ln":
		sc.transfer(name, args)
	case name == "dd":
		for _, a := range args {
			if v, ok := strings.CutPrefix(a.s, "of="); ok {
				sc.add(shWord{s: v, glob: a.glob, dyn: a.dyn, loop: a.loop}, true, "dd")
			}
		}
	}
}

func skipOptions(words []shWord, withArg map[string]bool) []shWord {
	for len(words) > 0 {
		w := words[0]
		switch {
		case withArg[w.s]:
			words = words[min(2, len(words)):]
		case strings.HasPrefix(w.s, "-") && len(w.s) > 1:
			words = words[1:]
		default:
			return words
		}
	}
	return words
}

func (sc *shScanner) transfer(name string, args []shWord) {
	var target *shWord
	var plain []shWord
	rest := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !rest && a.s == "--":
			rest = true
		case !rest && (a.s == "-t" || a.s == "--target-directory") && i+1 < len(args):
			target = &args[i+1]
			i++
		case !rest && strings.HasPrefix(a.s, "--target-directory="):
			w := shWord{s: strings.TrimPrefix(a.s, "--target-directory="), glob: a.glob, dyn: a.dyn, loop: a.loop}
			target = &w
		case !rest && strings.HasPrefix(a.s, "-") && len(a.s) > 1:
		default:
			plain = append(plain, a)
		}
	}
	var srcs []shWord
	var dest shWord
	switch {
	case target != nil:
		srcs, dest = plain, *target
	case len(plain) >= 2:
		srcs, dest = plain[:len(plain)-1], plain[len(plain)-1]
	default:
		return
	}
	if name == "mv" {
		for _, s := range srcs {
			sc.add(s, true, "mv")
		}
	}
	if dest.dyn || dest.glob {
		return
	}
	if strings.HasSuffix(dest.s, ".md") && target == nil {
		sc.add(dest, true, name)
		return
	}
	for _, s := range srcs {
		if s.dyn || s.glob {
			continue
		}
		sc.add(shWord{s: strings.TrimRight(dest.s, "/") + "/" + filepath.Base(s.s), loop: dest.loop || s.loop}, true, name)
	}
}

type optionSpec struct {
	short   string
	long    map[string]bool
	pattern string
	file    string
}

var optionSpecs = map[string]optionSpec{
	"rg": {short: "ABCefgmtTEjdMrd", pattern: "ef", file: "f", long: map[string]bool{"--regexp": true, "--file": true, "--glob": true, "--iglob": true, "--type": true, "--type-not": true,
		"--type-add": true, "--max-count": true, "--max-depth": true, "--maxdepth": true, "--max-filesize": true, "--context": true, "--after-context": true, "--before-context": true,
		"--replace": true, "--threads": true, "--encoding": true, "--colors": true, "--engine": true, "--pre": true, "--pre-glob": true, "--sort": true, "--sortr": true,
		"--ignore-file": true, "--path-separator": true, "--max-columns": true, "--regex-size-limit": true, "--dfa-size-limit": true}},
	"grep": {short: "ABCefmdD", pattern: "ef", file: "f", long: map[string]bool{"--regexp": true, "--file": true, "--max-count": true, "--after-context": true, "--before-context": true,
		"--context": true, "--directories": true, "--devices": true, "--include": true, "--exclude": true, "--exclude-from": true, "--exclude-dir": true, "--label": true, "--binary-files": true}},
	"sed":  {short: "efl", pattern: "e", file: "f", long: map[string]bool{"--expression": true, "--file": true, "--line-length": true}},
	"awk":  {short: "Fvfie", pattern: "fe", file: "", long: map[string]bool{"--field-separator": true, "--assign": true, "--file": true, "--include": true, "--source": true}},
	"perl": {short: "eEIMm", pattern: "eE", file: "", long: nil},
	"ruby": {short: "eIrC", pattern: "e", file: "", long: nil},
}

var optionAliases = map[string]string{
	"egrep": "grep", "fgrep": "grep", "ag": "rg", "ack": "rg", "gawk": "awk", "mawk": "awk", "nawk": "awk",
}

func patternArgs(name string, args []shWord) []shWord {
	if a, ok := optionAliases[name]; ok {
		name = a
	}
	spec, ok := optionSpecs[name]
	if !ok {
		return operandsOf(args)
	}
	var files, positional []shWord
	hasPattern, rest := false, false
	for i := 0; i < len(args); i++ {
		w := args[i]
		switch {
		case rest:
			positional = append(positional, w)
		case w.s == "--" && !w.dyn:
			rest = true
		case strings.HasPrefix(w.s, "--") && len(w.s) > 2:
			opt, val, hasVal := strings.Cut(w.s, "=")
			if spec.long[opt] || opt == "--regexp" || opt == "--expression" || opt == "--file" {
				if opt == "--regexp" || opt == "--expression" || (name == "awk" && opt == "--source") {
					hasPattern = true
				}
				if opt == "--file" && name != "awk" {
					if hasVal {
						files = append(files, shWord{s: val, glob: w.glob, dyn: w.dyn, loop: w.loop})
					} else if i+1 < len(args) {
						files = append(files, args[i+1])
					}
				}
				if opt == "--file" && name == "awk" {
					hasPattern = true
				}
				if !hasVal {
					i++
				}
			}
		case strings.HasPrefix(w.s, "-") && len(w.s) > 1:
			cluster := w.s[1:]
			for j := 0; j < len(cluster); j++ {
				c := cluster[j]
				if name == "sed" && c == 'i' {
					break
				}
				if !strings.ContainsRune(spec.short, rune(c)) {
					continue
				}
				if strings.ContainsRune(spec.pattern, rune(c)) {
					hasPattern = true
				}
				isFile := spec.file != "" && strings.ContainsRune(spec.file, rune(c))
				if j+1 < len(cluster) {
					if isFile {
						files = append(files, shWord{s: cluster[j+1:], glob: w.glob, dyn: w.dyn, loop: w.loop})
					}
					break
				}
				if i+1 < len(args) {
					if isFile {
						files = append(files, args[i+1])
					}
					i++
				}
				break
			}
		default:
			positional = append(positional, w)
		}
	}
	if !hasPattern && len(positional) > 0 && name != "perl" && name != "ruby" {
		positional = positional[1:]
	}
	return append(files, positional...)
}

func (sc *shScanner) fileArgs(args []shWord, write bool, op, name string) {
	if !write {
		op = name
	}
	for _, o := range patternArgs(name, args) {
		sc.add(o, write, op)
	}
}

func (sc *shScanner) forHeader(words []shWord) {
	if len(words) < 2 || words[0].dyn || words[0].glob || words[0].loop || varNameRe.FindString(words[0].s) != words[0].s || words[1].s != "in" {
		if len(words) > 0 {
			delete(sc.loops, words[0].s)
		}
		return
	}
	name := words[0].s
	var vals []string
	for _, w := range words[2:] {
		got := sc.values(w)
		if len(got) == 0 || w.dyn || w.glob {
			delete(sc.loops, name)
			return
		}
		vals = append(vals, got...)
	}
	if len(vals) == 0 || len(vals) > maxLoopWords {
		delete(sc.loops, name)
		return
	}
	sc.loops[name] = vals
	delete(sc.vars, name)
}

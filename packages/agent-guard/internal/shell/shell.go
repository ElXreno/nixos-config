// Package shell turns a Bash tool command into the list of simple commands it
// will run, including the ones handed to other interpreters through ssh, sh -c,
// container exec, heredocs and command substitutions.
package shell

import (
	"bytes"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type Kind int

const (
	Local Kind = iota
	// Remote marks commands that run where the Read tool and rg cannot follow:
	// another host, a container, another namespace.
	Remote
)

func (k Kind) String() string {
	if k == Remote {
		return "remote"
	}
	return "local"
}

type Origin int

const (
	Parsed Origin = iota
	Compound
	Fallback
)

func (o Origin) String() string {
	switch o {
	case Compound:
		return "compound"
	case Fallback:
		return "fallback"
	}
	return "parsed"
}

// Command is one simple command with its words as the program will see them.
type Command struct {
	Kind       Kind
	Origin     Origin
	Argv       []string
	Words      []string
	Raw        string
	Background bool
	Detached   bool
	InPipeline bool
	Redirects  []Redirect
	Heredoc    string
	Stdin      string
	Depth      int
	Env        map[string]string

	unwrapped []string
	ready     bool
}

func (c *Command) settle() {
	var heads []string
	c.unwrapped, heads = unwrap(c.Argv)
	c.ready = true
	c.Detached = c.Detached || detachedBy(heads, c.unwrapped)
}

func (c Command) effective() []string {
	if c.ready {
		return c.unwrapped
	}
	return Unwrap(c.Argv)
}

type Redirect struct {
	Op     string
	Target string
}

// Head returns the effective program after wrapper prefixes such as sudo, env
// and timeout are stripped.
func (c Command) Head() string {
	argv := c.effective()
	if len(argv) == 0 {
		return ""
	}
	return base(argv[0])
}

// Args returns the effective arguments after wrapper prefixes.
func (c Command) Args() []string {
	argv := c.effective()
	if len(argv) == 0 {
		return nil
	}
	return argv[1:]
}

const (
	ReasonParse = "parse"
	ReasonDepth = "depth"
)

type Unparsed struct {
	Text   string
	Kind   Kind
	Depth  int
	Err    error
	Reason string
}

type Analysis struct {
	Source    string
	Commands  []Command
	Unparsed  []Unparsed
	seenTexts map[string]bool
	// env holds the literal values the command assigns, in source order.
	env      map[string]string
	exported map[string]bool
}

const MaxDepth = 4

// Analyze parses src as bash and returns every simple command reachable from it.
func Analyze(src string) *Analysis {
	a := &Analysis{Source: src, seenTexts: map[string]bool{}, env: map[string]string{}, exported: map[string]bool{}}
	a.add(src, Local, 0)
	return a
}

var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expand substitutes the variables whose values are known: those the command
// assigned itself, then the hook's own environment, which the Bash tool shares.
// Anything else stays as written, so callers can still tell it is unresolved.
func (a *Analysis) expand(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return varRef.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.Trim(m[1:], "{}")
		if v, ok := a.env[name]; ok {
			return v
		}
		if name == "PWD" || name == "OLDPWD" {
			return m
		}
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return m
	})
}

// record learns from assignments that run as statements of their own; a prefix
// assignment on a command only reaches that command's environment and is
// handled by the caller not calling this.
func (a *Analysis) record(assigns []*syntax.Assign) {
	for _, as := range assigns {
		if as.Name == nil {
			continue
		}
		name := as.Name.Value
		if as.Array != nil || as.Append || as.Value == nil {
			delete(a.env, name)
			continue
		}
		value := Flatten(as.Value)
		if strings.Contains(value, "$(") || strings.Contains(value, "`") {
			delete(a.env, name)
			continue
		}
		value = a.expand(value)
		if strings.Contains(value, "$") {
			delete(a.env, name)
			continue
		}
		a.env[name] = value
	}
}

func (a *Analysis) export(d *syntax.DeclClause) {
	exports := d.Variant != nil && d.Variant.Value == "export"
	for _, as := range d.Args {
		if as.Name == nil && as.Value != nil {
			if flag := Flatten(as.Value); strings.HasPrefix(flag, "-") && strings.Contains(flag, "x") {
				exports = true
			}
		}
	}
	if !exports {
		return
	}
	for _, as := range d.Args {
		if as.Name != nil {
			a.exported[as.Name.Value] = true
		}
	}
}

func (a *Analysis) commandEnv(assigns []*syntax.Assign) map[string]string {
	var env map[string]string
	set := func(name, value string) {
		if env == nil {
			env = map[string]string{}
		}
		env[name] = value
	}
	for name := range a.exported {
		if v, ok := a.env[name]; ok {
			set(name, v)
		} else {
			set(name, "$"+name)
		}
	}
	for _, as := range assigns {
		if as.Name == nil {
			continue
		}
		if as.Value == nil || as.Array != nil || as.Append {
			set(as.Name.Value, "$"+as.Name.Value)
			continue
		}
		set(as.Name.Value, a.expand(Flatten(as.Value)))
	}
	return env
}

func delimiterQuoted(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, p := range w.Parts {
		switch x := p.(type) {
		case *syntax.SglQuoted, *syntax.DblQuoted:
			return true
		case *syntax.Lit:
			if strings.Contains(x.Value, `\`) {
				return true
			}
		}
	}
	return false
}

func (a *Analysis) heredocBody(r *syntax.Redirect) string {
	quoted := delimiterQuoted(r.Word)
	var b strings.Builder
	for _, p := range r.Hdoc.Parts {
		lit, ok := p.(*syntax.Lit)
		switch {
		case ok && quoted:
			b.WriteString(lit.Value)
		case ok:
			b.WriteString(heredocUnescape(lit.Value))
		default:
			b.WriteString(Source(p))
		}
	}
	if quoted {
		return b.String()
	}
	return a.expand(b.String())
}

func heredocUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("$`\\\n", s[i+1]) >= 0 {
			if s[i+1] != '\n' {
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (a *Analysis) add(src string, kind Kind, depth int) {
	if strings.TrimSpace(src) == "" {
		return
	}
	if depth > MaxDepth {
		a.Unparsed = append(a.Unparsed, Unparsed{Text: src, Kind: kind, Depth: depth, Err: errors.New("nested more than " + strconv.Itoa(MaxDepth) + " interpreters deep"), Reason: ReasonDepth})
		return
	}
	key := kind.String() + "\x00" + src
	if a.seenTexts[key] {
		return
	}
	a.seenTexts[key] = true

	file, err := parseStrict(src)
	if err != nil {
		a.Unparsed = append(a.Unparsed, Unparsed{Text: src, Kind: kind, Depth: depth, Err: err, Reason: ReasonParse})
		a.Commands = append(a.Commands, fallbackCommands(src, kind, depth)...)
		file = parseRecover(src)
		if file == nil {
			return
		}
	}

	if inner, ok := wholeQuoted(file); ok {
		a.add(inner, kind, depth)
		return
	}

	var stack []syntax.Node
	piped := map[*syntax.Stmt]string{}
	syntax.Walk(file, func(n syntax.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		if bc, ok := n.(*syntax.BinaryCmd); ok && bc.Op == syntax.Pipe {
			if text, ok := a.printed(bc.X); ok {
				piped[pipeHead(bc.Y)] = text
			}
		}
		stmt, ok := n.(*syntax.Stmt)
		if !ok {
			return true
		}
		switch c := stmt.Cmd.(type) {
		case *syntax.DeclClause:
			a.record(c.Args)
			a.export(c)
		case *syntax.CallExpr:
			if len(c.Args) == 0 {
				a.record(c.Assigns)
			}
		}
		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			if stmt.Background {
				a.Commands = append(a.Commands, Command{
					Kind:       kind,
					Origin:     Compound,
					Background: true,
					Raw:        strings.TrimSpace(slice(src, stmt)),
					Depth:      depth,
				})
			}
			return true
		}
		cmd := Command{
			Kind:       kind,
			Background: stmt.Background,
			InPipeline: inPipeline(stack),
			Raw:        strings.TrimSpace(slice(src, stmt)),
			Depth:      depth,
			Env:        a.commandEnv(call.Assigns),
			Stdin:      piped[stmt],
		}
		for _, w := range call.Args {
			cmd.Argv = append(cmd.Argv, a.expand(Flatten(w)))
			cmd.Words = append(cmd.Words, slice(src, w))
		}
		for _, r := range stmt.Redirs {
			rd := Redirect{Op: r.Op.String()}
			if r.Word != nil {
				rd.Target = a.expand(Flatten(r.Word))
			}
			if r.Hdoc != nil {
				cmd.Heredoc += a.heredocBody(r)
			}
			if r.Op == syntax.WordHdoc {
				cmd.Stdin += rd.Target + "\n"
			}
			cmd.Redirects = append(cmd.Redirects, rd)
		}
		cmd.settle()
		a.Commands = append(a.Commands, cmd)
		for _, p := range Payloads(cmd) {
			a.add(p.Text, p.Kind, depth+1)
		}
		return true
	})
}

var unclosedHeredoc = regexp.MustCompile("^unclosed here-document `([^`]+)`")

// parseStrict parses bash exactly, closing an unterminated heredoc at end of input as bash does.
func parseStrict(src string) (*syntax.File, error) {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		var file *syntax.File
		file, err = syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
		if err == nil {
			return file, nil
		}
		var pe syntax.ParseError
		if !errors.As(err, &pe) {
			return nil, err
		}
		m := unclosedHeredoc.FindStringSubmatch(pe.Text)
		if m == nil {
			return nil, err
		}
		src = strings.TrimRight(src, "\n") + "\n" + m[1] + "\n"
	}
	return nil, err
}

func parseRecover(src string) *syntax.File {
	p := syntax.NewParser(syntax.Variant(syntax.LangBash), syntax.RecoverErrors(5))
	file, err := p.Parse(strings.NewReader(src), "")
	if file == nil || (err != nil && len(file.Stmts) == 0) {
		return nil
	}
	return file
}

// wholeQuoted returns the text of a payload that is one quoted string, the
// shape `bash -c "'...'"` leaves once ssh strips the outer quotes.
func wholeQuoted(file *syntax.File) (string, bool) {
	if len(file.Stmts) != 1 {
		return "", false
	}
	call, ok := file.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 1 || len(call.Assigns) != 0 || len(file.Stmts[0].Redirs) != 0 {
		return "", false
	}
	w := call.Args[0]
	quoted := false
	for _, p := range w.Parts {
		switch x := p.(type) {
		case *syntax.SglQuoted, *syntax.DblQuoted:
			quoted = true
		case *syntax.Lit:
			if strings.Trim(x.Value, `\'"`) != "" {
				return "", false
			}
		default:
			return "", false
		}
	}
	if !quoted {
		return "", false
	}
	inner := Flatten(w)
	if strings.ContainsAny(inner, " \t\n;|&") {
		return inner, true
	}
	return "", false
}

func inPipeline(stack []syntax.Node) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		if bc, ok := stack[i].(*syntax.BinaryCmd); ok {
			if bc.Op == syntax.Pipe || bc.Op == syntax.PipeAll {
				return true
			}
			continue
		}
		if _, ok := stack[i].(*syntax.Stmt); ok {
			continue
		}
		return false
	}
	return false
}

// Flatten renders a word the way the command will see it: quotes removed,
// backslash escapes resolved, expansions left as their source text.
func Flatten(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		flattenPart(&b, p, false)
	}
	return b.String()
}

func flattenPart(b *strings.Builder, p syntax.WordPart, inDouble bool) {
	switch x := p.(type) {
	case *syntax.Lit:
		b.WriteString(unescape(x.Value, inDouble))
	case *syntax.SglQuoted:
		b.WriteString(x.Value)
	case *syntax.DblQuoted:
		for _, q := range x.Parts {
			flattenPart(b, q, true)
		}
	default:
		b.WriteString(Source(p))
	}
}

func unescape(s string, inDouble bool) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		next := s[i+1]
		if inDouble && !strings.ContainsRune("\"\\$`\n", rune(next)) {
			b.WriteByte('\\')
			continue
		}
		if next == '\n' {
			i++
			continue
		}
		b.WriteByte(next)
		i++
	}
	return b.String()
}

// Source prints a node back as shell text.
func Source(n syntax.Node) string {
	var buf bytes.Buffer
	if err := syntax.NewPrinter().Print(&buf, n); err != nil {
		return ""
	}
	return buf.String()
}

// slice returns the node's original text. Nodes the parser synthesised while
// recovering from an error carry no positions, so those are printed instead.
func slice(src string, n syntax.Node) string {
	start, end := n.Pos(), n.End()
	if !start.IsValid() || !end.IsValid() {
		return Source(n)
	}
	a, b := int(start.Offset()), int(end.Offset())
	if a < 0 || b > len(src) || a > b {
		return Source(n)
	}
	return src[a:b]
}

func base(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

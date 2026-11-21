package shell

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func pipeHead(s *syntax.Stmt) *syntax.Stmt {
	for s != nil {
		bc, ok := s.Cmd.(*syntax.BinaryCmd)
		if !ok || (bc.Op != syntax.Pipe && bc.Op != syntax.PipeAll) {
			return s
		}
		s = bc.X
	}
	return s
}

func echoFlags(w string) bool {
	return len(w) > 1 && w[0] == '-' && strings.Trim(w[1:], "neE") == ""
}

func (a *Analysis) printed(s *syntax.Stmt) (string, bool) {
	if s == nil {
		return "", false
	}
	call, ok := s.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", false
	}
	argv := make([]string, len(call.Args))
	for i, w := range call.Args {
		argv[i] = a.expand(Flatten(w))
	}
	args := argv[1:]
	switch argv[0] {
	case "echo":
		escapes := false
		for len(args) > 0 && echoFlags(args[0]) {
			if i := strings.LastIndexAny(args[0], "eE"); i > 0 {
				escapes = args[0][i] == 'e'
			}
			args = args[1:]
		}
		text := strings.Join(args, " ")
		if escapes {
			text = printfEscapes(text)
		}
		return text + "\n", true
	case "printf":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 || strings.HasPrefix(args[0], "-v") {
			return "", false
		}
		return printfFormat(args[0], args[1:]), true
	}
	return "", false
}

var printfEscape = map[byte]string{'n': "\n", 't': "\t", 'r': "\r", '\\': "\\", '"': "\"", '\'': "'", 'a': "\a", 'b': "\b", 'f': "\f", 'v': "\v", 'e': "\x1b", 'E': "\x1b"}

func printfEscapes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if r, ok := printfEscape[s[i+1]]; ok {
				b.WriteString(r)
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func printfFormat(format string, args []string) string {
	var b strings.Builder
	for pass := 0; pass < 64; pass++ {
		consumed := false
		next := func() string {
			consumed = true
			if len(args) == 0 {
				return ""
			}
			v := args[0]
			args = args[1:]
			return v
		}
		for i := 0; i < len(format); i++ {
			c := format[i]
			switch {
			case c == '\\' && i+1 < len(format):
				if r, ok := printfEscape[format[i+1]]; ok {
					b.WriteString(r)
					i++
					continue
				}
				b.WriteByte(c)
			case c == '%' && i+1 < len(format) && format[i+1] == '%':
				b.WriteByte('%')
				i++
			case c == '%':
				j := i + 1
				for j < len(format) && strings.IndexByte("-+ #0123456789.*", format[j]) >= 0 {
					if format[j] == '*' {
						next()
					}
					j++
				}
				if j >= len(format) {
					b.WriteString(format[i:])
					i = j
					continue
				}
				v := next()
				if format[j] == 'b' {
					v = printfEscapes(v)
				}
				b.WriteString(v)
				i = j
			default:
				b.WriteByte(c)
			}
		}
		if len(args) == 0 || !consumed {
			break
		}
	}
	return b.String()
}

package treesitter

import (
	"fmt"
	"regexp"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

const NixValue = "NIXVALUE"

type Snippet struct {
	Start, End int
	Text       string
}

var hookAttr = regexp.MustCompile(`^(?:\w+Phase|(?:pre|post)[A-Z]\w*|script|shellHook|buildCommand|extraCommands|initExtra|shellInit|interactiveShellInit|loginShellInit)$`)

func shellWriter(name string) bool {
	for _, p := range []string{"writeShell", "writeBash", "writeScript", "writeDash", "runCommand"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func (g Grammars) ShellSnippets(path string, src []byte) ([]Snippet, error) {
	if GrammarFor(path) != "nix" {
		return nil, nil
	}
	l, err := g.Load("nix")
	if err != nil {
		return nil, err
	}
	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(l); err != nil {
		return nil, fmt.Errorf("grammar nix: %w", err)
	}
	tree := p.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("grammar nix: the parser returned no tree")
	}
	defer tree.Close()
	var out []Snippet
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Kind() == "indented_string_expression" && holdsShell(n, src) {
			out = append(out, Snippet{int(n.StartByte()), int(n.EndByte()), decodeIndented(n, src)})
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())
	return out, nil
}

func text(n *sitter.Node, src []byte) string { return string(src[n.StartByte():n.EndByte()]) }

func holdsShell(s *sitter.Node, src []byte) bool {
	parent := s.Parent()
	if parent == nil {
		return false
	}
	switch parent.Kind() {
	case "apply_expression":
		return shellWriter(applyHead(parent, src))
	case "binding":
		attr := lastAttr(parent, src)
		if attr == "text" {
			set := parent.Parent()
			if set != nil {
				set = set.Parent()
			}
			if set == nil || !strings.Contains(set.Kind(), "attrset") || set.Parent() == nil || set.Parent().Kind() != "apply_expression" {
				return false
			}
			return shellWriter(applyHead(set.Parent(), src))
		}
		return hookAttr.MatchString(attr)
	}
	return false
}

func lastAttr(binding *sitter.Node, src []byte) string {
	for i := uint(0); i < binding.ChildCount(); i++ {
		c := binding.Child(i)
		if c.Kind() == "attrpath" && c.ChildCount() > 0 {
			return text(c.Child(c.ChildCount()-1), src)
		}
	}
	return ""
}

func applyHead(n *sitter.Node, src []byte) string {
	for n != nil && n.Kind() == "apply_expression" && n.ChildCount() > 0 {
		n = n.Child(0)
	}
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "variable_expression":
		return text(n, src)
	case "select_expression":
		for i := int(n.ChildCount()) - 1; i >= 0; i-- {
			c := n.Child(uint(i))
			if c.Kind() == "attrpath" && c.ChildCount() > 0 {
				return text(c.Child(c.ChildCount()-1), src)
			}
		}
	}
	return ""
}

func decodeIndented(s *sitter.Node, src []byte) string {
	var b strings.Builder
	for i := uint(0); i < s.ChildCount(); i++ {
		c := s.Child(i)
		t := text(c, src)
		switch c.Kind() {
		case "string_fragment":
			b.WriteString(t)
		case "interpolation":
			b.WriteString(NixValue)
		case "escape_sequence":
			switch {
			case t == "'''":
				b.WriteString("''")
			case strings.HasPrefix(t, `''\`) && len(t) == 4:
				switch t[3] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(t[3])
				}
			}
		}
	}
	return b.String()
}

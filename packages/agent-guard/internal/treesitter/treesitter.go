// Package treesitter finds comments with tree-sitter. The grammars are the parsers
// nixpkgs builds; the guard configuration maps each grammar name to its shared
// object, and a grammar is loaded only when a file of its language is edited.
package treesitter

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"agent-guard/internal/lang"
)

type Grammars map[string]string

// embedded are multi-line strings that usually carry another language, a
// shell script above all; the caller keeps its line heuristic inside them.
func embedded(kind string) bool {
	return kind == "indented_string_expression" || kind == "block_scalar" || strings.Contains(kind, "heredoc")
}

func GrammarFor(path string) string {
	return lang.Of(path).Grammar
}

func FromFile(path string) (Grammars, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g Grammars
	return g, json.Unmarshal(data, &g)
}

type loadResult struct {
	lang *sitter.Language
	err  error
}

var loaded sync.Map

func (g Grammars) Load(name string) (*sitter.Language, error) {
	path, ok := g[name]
	if !ok {
		return nil, fmt.Errorf("grammar %s is not configured", name)
	}
	if r, ok := loaded.Load(path); ok {
		res := r.(loadResult)
		return res.lang, res.err
	}
	l, err := dlopen(path, name)
	if err != nil {
		err = fmt.Errorf("grammar %s: %w", name, err)
	}
	loaded.Store(path, loadResult{l, err})
	return l, err
}

type Region struct{ Start, End int }

type Tree struct {
	Comments []Region
	Embedded []Region
	Errors   []Region
}

// Parse returns the comments, embedded strings and syntax errors of src; a nil
// tree without an error means no grammar covers the file.
func (g Grammars) Parse(path string, src []byte) (*Tree, error) {
	name := GrammarFor(path)
	if name == "" {
		return nil, nil
	}
	lang, err := g.Load(name)
	if err != nil {
		return nil, err
	}
	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("grammar %s: %w", name, err)
	}
	tree := p.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("grammar %s: the parser returned no tree", name)
	}
	defer tree.Close()
	out := &Tree{}
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		r := Region{int(n.StartByte()), int(n.EndByte())}
		kind := n.Kind()
		switch {
		case n.IsError() || n.IsMissing():
			out.Errors = append(out.Errors, r)
		case strings.Contains(kind, "comment") || kind == "haddock":
			out.Comments = append(out.Comments, r)
			return
		case embedded(kind):
			out.Embedded = append(out.Embedded, r)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())
	return out, nil
}

func overlaps(rs []Region, start, end int) bool {
	for _, r := range rs {
		if r.Start < end && start < r.End {
			return true
		}
	}
	return false
}

func (t *Tree) ErrorIn(start, end int) bool    { return overlaps(t.Errors, start, end) }
func (t *Tree) EmbeddedIn(start, end int) bool { return overlaps(t.Embedded, start, end) }

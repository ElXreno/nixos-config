package edit

import (
	"regexp"
	"strings"
	"testing"

	"agent-guard/internal/call"
	"agent-guard/internal/treesitter"
	"agent-guard/internal/verdict"
)

func fixed(text string) Options {
	return Options{Read: func(string) (string, bool) { return text, true }}
}

func TestApplyOriginal(t *testing.T) {
	swap := edits("/r/x.txt", call.Pair{Old: "a", New: "b"}, call.Pair{Old: "b", New: "c"})
	if _, _, _, ok := applyEdit(swap, fixed("a\nb\n")); ok {
		t.Error("sequential: the second pair matches twice after the first, the tool would refuse")
	}
	swap.Apply = call.Original
	text, spans, olds, ok := applyEdit(swap, fixed("a\nb\n"))
	if !ok || text != "b\nc\n" {
		t.Fatalf("original: got %q, %v", text, ok)
	}
	if len(spans) != 2 || spans[0] != (span{0, 1, 0}) || spans[1] != (span{2, 3, 1}) {
		t.Errorf("original: spans %v", spans)
	}
	if len(olds) != 2 || olds[0] != "a" || olds[1] != "b" {
		t.Errorf("original: olds %q", olds)
	}

	cases := []struct {
		name  string
		pairs []call.Pair
	}{
		{"overlap", []call.Pair{{Old: "ab", New: "x"}, {Old: "bc", New: "y"}}},
		{"miss", []call.Pair{{Old: "a", New: "x"}, {Old: "zz", New: "y"}}},
		{"ambiguous", []call.Pair{{Old: "c", New: "x"}}},
		{"empty old", []call.Pair{{Old: "", New: "x"}}},
	}
	for _, tc := range cases {
		c := call.Change{Path: "/r/x.txt", Pairs: tc.pairs, Apply: call.Original}
		if _, _, _, ok := applyEdit(c, fixed("abc c\n")); ok {
			t.Errorf("%s: original must refuse", tc.name)
		}
	}
}

func TestApplyOriginalMatchesLikePi(t *testing.T) {
	c := call.Change{Path: "/r/x.txt", Apply: call.Original, Pairs: []call.Pair{{Old: "a\nb", New: "x\r\ny"}}}
	text, spans, _, ok := applyEdit(c, fixed("\uFEFFa\r\nb\r\nc\r\n"))
	if !ok || text != "x\ny\nc\n" || len(spans) != 1 || spans[0] != (span{0, 3, 0}) {
		t.Errorf("CRLF, a lone CR and a BOM are normalized as pi does: got %q %v %v", text, spans, ok)
	}
	c.Apply = call.Sequential
	if _, _, _, ok := applyEdit(c, fixed("a\r\nb\r\n")); ok {
		t.Error("sequential matching stays exact")
	}
}

func TestApplyOriginalFeedsTheParser(t *testing.T) {
	g := testGrammars(t)
	src := "package x\n\nfunc f() {\n\tone()\n\ttwo()\n}\n"
	story := "// We stopped the workers on 16.08 because the queue\n\t// had 5000 duplicates; 44% were the same job.\n\t// Now we drain them.\n\t"
	c := call.Change{Path: "/r/x.go", Apply: call.Original, Pairs: []call.Pair{
		{Old: "one()", New: story + "one()"},
		{Old: "two()", New: "three()"},
	}}
	for _, grammars := range []treesitter.Grammars{nil, g} {
		opts := fixed(src)
		opts.Grammars = grammars
		if got := Comments(c, opts).Worst(); got != verdict.Deny {
			t.Errorf("grammars %v: want deny got %v", grammars != nil, got)
		}
	}
}

func addedComments(lines []line) int {
	n := 0
	for _, l := range lines {
		if l.added && l.comment != "" {
			n++
		}
	}
	return n
}

func TestKeptLinesAreCountedOnce(t *testing.T) {
	src := "package x\n\n// keep this note\nvar x = 1\n"
	c := edit("/r/x.go", "// keep this note\nvar x = 1", "// keep this note\n// keep this note\n// keep this note\n// keep this note\nvar x = 2")
	lang, _ := languageOf(c.Path)
	if got := addedComments(editedLines(c, lang, Options{})); got != 3 {
		t.Errorf("heuristic: one copy was there before, three are new, got %d new", got)
	}
	opts := fixed(src)
	opts.Grammars = testGrammars(t)
	lines, ok := parsedLines(c, lang, opts)
	if !ok {
		t.Fatal("no parse")
	}
	if got := addedComments(lines); got != 3 {
		t.Errorf("syntax: one copy was there before, three are new, got %d new", got)
	}
}

func TestEditDenialsQuoteInsteadOfNumbering(t *testing.T) {
	story := "# We moved the cache here after the duplicates,\n# and the numbers dropped by 44%,\n# so keep it.\n"
	e := Comments(edit("/r/x.py", "x = 1", story+"x = 1"), Options{})
	w := Comments(write("/r/x.py", "import os\n"+story+"x = 1\n"), Options{})
	if len(e) == 0 || len(w) == 0 {
		t.Fatal("both must be denied")
	}
	if regexp.MustCompile(`\bL[0-9]`).MatchString(e[0].Message) || !strings.Contains(e[0].Message, `"# We moved the cache here`) {
		t.Errorf("an edit cites quoted samples, the file it would produce does not exist yet: %s", e[0].Message)
	}
	if !strings.Contains(w[0].Message, "L2-L4") {
		t.Errorf("a write cites line numbers: %s", w[0].Message)
	}
}

func TestNotes(t *testing.T) {
	var notes []string
	opts := fixed("package x\n")
	opts.Note = func(n string) { notes = append(notes, n) }
	opts.Grammars = treesitter.Grammars{"go": "/nonexistent/libtree-sitter-go.so"}
	Comments(call.Change{Path: "/r/x.go", Apply: call.Original, Pairs: []call.Pair{{Old: "missing", New: "y"}}}, opts)
	Comments(write("/r/x.go", "package x\n"), opts)
	if len(notes) != 2 || !strings.Contains(notes[0], "no exact match") || !strings.Contains(notes[1], "grammar go") {
		t.Errorf("want a match note and a grammar note, got %q", notes)
	}
}

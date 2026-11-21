package treesitter

import (
	"strings"
	"testing"

	"agent-guard/internal/lang"
	"agent-guard/internal/treesitter/treesittertest"
)

func testGrammars(t *testing.T) Grammars {
	t.Helper()
	g, err := FromFile(treesittertest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestEveryGrammarLoads(t *testing.T) {
	g := testGrammars(t)
	for name := range g {
		if _, err := g.Load(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range lang.Grammars() {
		if _, ok := g[name]; !ok {
			t.Errorf("the language table names %s, which the configuration does not ship", name)
		}
	}
}

func TestParse(t *testing.T) {
	g := testGrammars(t)
	cases := []struct {
		path, src string
		comments  int
		embedded  bool
	}{
		{"x.nix", "{ s = ''\n  #define X\n''; } # real\n", 1, true},
		{"x.py", "s = \"\"\"\n# no\n\"\"\"\nx = 1  # yes\n", 1, false},
		{"x.go", "package x\n\nvar s = `\n// no\n`\n\n// yes\nfunc f() {}\n", 1, false},
		{"x.yaml", "run: |\n  # inside\nb: 1 # yes\n", 1, true},
		{"x.sql", "-- yes\nSELECT '-- no';\n", 1, false},
		{"Dockerfile", "# yes\nRUN echo '# no'\n", 1, false},
	}
	for _, c := range cases {
		tree, err := g.Parse(c.path, []byte(c.src))
		if tree == nil || err != nil {
			t.Errorf("%s: no parse: %v", c.path, err)
			continue
		}
		if len(tree.Comments) != c.comments || (len(tree.Embedded) > 0) != c.embedded {
			t.Errorf("%s: want %d comments, embedded %v; got %d, %v", c.path, c.comments, c.embedded, len(tree.Comments), len(tree.Embedded) > 0)
		}
	}
	if tree, err := g.Parse("x.unknown", nil); tree != nil || err != nil {
		t.Errorf("an unknown extension has no grammar and no error: %v %v", tree, err)
	}
}

func TestLoadErrorsAreReportedAndCached(t *testing.T) {
	g := Grammars{"go": "/nonexistent/libtree-sitter-go.so"}
	_, err := g.Parse("x.go", []byte("package x"))
	if err == nil || !strings.Contains(err.Error(), "grammar go") {
		t.Fatalf("want the load error, got %v", err)
	}
	if _, again := g.Parse("y.go", nil); again == nil || again.Error() != err.Error() {
		t.Errorf("the failure is cached per path: %v", again)
	}
	if _, err := (Grammars{}).Parse("x.go", nil); err == nil {
		t.Error("an unconfigured grammar is an error")
	}
}

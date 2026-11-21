package edit

import (
	"testing"
	"unicode/utf8"

	"agent-guard/internal/call"
	"agent-guard/internal/verdict"
)

func TestAIMarkers(t *testing.T) {
	cases := []struct {
		name string
		in   call.Change
		want verdict.Level
	}{
		{"memory file exempt", write("/home/u/.claude/projects/-x/memory/reference_foo.md", "a — b"), verdict.Silent},
		{"memory index exempt", write("/home/u/.claude/projects/-x/memory/MEMORY.md", "a — b"), verdict.Silent},
		{"agent memory exempt", write("/repo/.claude/agent-memory-local/log-forensics/notes.md", "a — b"), verdict.Silent},
		{"em-dash in repo file", write("/home/u/repo/README.md", "a — b"), verdict.Advise},
		{"clean edit", edit("/home/u/repo/README.md", "", "a - b"), verdict.Silent},
		{"marker carried over from old_string", edit("/home/u/repo/README.md", "keep — this\nold", "keep — this\nnew"), verdict.Silent},
		{"curly quotes", edit("/home/u/repo/x.go", "", "say “hi”"), verdict.Advise},
		{"ellipsis character", edit("/home/u/repo/x.md", "", "wait"+string(rune(0x2026))), verdict.Advise},
		{"three dots are fine", edit("/home/u/repo/x.md", "", "wait..."), verdict.Silent},
		{"zero width", edit("/home/u/repo/x.go", "", "a"+string(rune(0x200B))+"b"), verdict.Advise},
		{"arrow is fine", edit("/home/u/repo/x.md", "", "a → b"), verdict.Silent},
		{"ren'py dialogue", edit("/home/u/game/script.rpy", "", "    m \"Well — fine.\""), verdict.Silent},
		{"translation catalog", edit("/home/u/app/ru.po", "", "msgstr \"Готово — можно ехать\""), verdict.Silent},
		{"dash trimming script", edit("/home/u/bin/trim_dashes.pl", "", "s/ — / - /g;"), verdict.Silent},
		{"sed substitution", edit("/home/u/bin/clean.sh", "", "sed -e 's/—/-/g' \"$f\""), verdict.Silent},
		{"perl transliteration", edit("/home/u/bin/clean.sh", "", "perl -pe 'tr/“”/\"\"/'"), verdict.Silent},
		{"regex character class", edit("/home/u/x.py", "", "s = re.sub(r\"[—–]\", \"-\", s)"), verdict.Silent},
		{"replacement table literal", edit("/home/u/x.py", "", "s = s.replace(\"—\", \"-\")"), verdict.Silent},
		{"emoji joined by a zero-width joiner", edit("/home/u/x.go", "", "icon := \"\U0001F468\u200d\U0001F4BB\""), verdict.Silent},
		{"string literal in a test file", edit("/home/u/x_test.go", "", "\twant := \"a — b\""), verdict.Silent},
		{"comment in a test file", edit("/home/u/x_test.go", "", "\t// a — b"), verdict.Advise},
		{"nix comment", edit("/home/u/x.nix", "", "  # a — b"), verdict.Advise},
		{"user-facing string", edit("/home/u/x.go", "", "\tlog.Printf(\"saved — %d rows\", n)"), verdict.Advise},
		{"quoted dash inside a comment", edit("/home/u/x.py", "", "# \"a — b\" is the label"), verdict.Advise},
		{"glyph between escaped quotes in a message", edit("/home/u/checks.go", "", "\tMessage: \"run it as fish -c \\\"…\\\" instead\","), verdict.Advise},
		{"glyph-only literal quoted in a comment", edit("/home/u/shell.go", "", "// shape `bash -c \"'…'\"` leaves behind"), verdict.Advise},
		{"docstring in a test file", edit("/home/u/tests/test_x.py", "", "    \"\"\"Недоступность брокера — транзиентная ошибка.\"\"\""), verdict.Advise},
		{"test string with escaped quotes", edit("/home/u/x_test.go", "", "\t{\"name\", In{S: \"m \\\"Well — fine.\\\"\"}},"), verdict.Silent},
		{"glyph between two quoted words", edit("/home/u/x.md", "", "He said \"yes\" — \"no\" to me"), verdict.Advise},
		{"lone zero-width joiner", edit("/home/u/x.py", "", "x = 1\u200d"), verdict.Advise},
		{"joiner between letters", edit("/home/u/x.py", "", "s = \"你\u200d好\""), verdict.Advise},
		{"yes-no prompt is not a substitution", edit("/home/u/x.md", "", "Continue (y/n) — default is no; see https://x.com"), verdict.Advise},
		{"url path is not a substitution", edit("/home/u/x.md", "", "Docs at https://example.com/tr/page — see https://github.com/a/b"), verdict.Advise},
		{"markdown link text is not a class", edit("/home/u/x.md", "", "see [Read—more](http://x) later"), verdict.Advise},
		{"citation range is not a class", edit("/home/u/x.md", "", "Refs [1–3] show it"), verdict.Advise},
		{"single glyph class in a regex string", edit("/home/u/x.py", "", "s = re.sub(r\"[—]\", \"-\", s)"), verdict.Silent},
		{"lone glyph literal in a call", edit("/home/u/x.go", "", "\tfmt.Println(\"name\", \"—\")"), verdict.Silent},
		{"prose file under tests", edit("/home/u/proj/tests/plan.md", "", "The \"foo — bar\" mode is documented here"), verdict.Advise},
		{"trailing comment in a test file", edit("/home/u/x_test.go", "", "\tx := f() // the \"foo — bar\" case"), verdict.Advise},
		{"prose after a sed call", edit("/home/u/bin/clean.sh", "", "sed -e 's/x/y/g' \"$f\" # swaps x — then y"), verdict.Advise},
	}
	match := MemoryMatcher([]string{"/home/u/.claude/projects/*/memory/", ".claude/agent-memory-local/"})
	opts := Options{Memory: func(p string) bool { return match(p, "/repo") }}
	for _, tc := range cases {
		if got := AIMarkers(tc.in, opts).Worst(); got != tc.want {
			t.Errorf("%s: want %v got %v", tc.name, tc.want, got)
		}
	}
	v := AIMarkers(edit("/home/u/repo/x.md", "", "xx Пропускная способность масштабируется только репликами "+string(rune(0x2014))+" вот"), Options{})
	if len(v) == 0 || !utf8.ValidString(v[0].Message) {
		t.Errorf("sample must be cut on a rune boundary: %q", v)
	}
}

func TestAIMarkersPerPair(t *testing.T) {
	cases := []struct {
		name string
		in   call.Change
		want verdict.Level
	}{
		{"marker in a later pair", edits("/r/x.md", call.Pair{Old: "x", New: "y"}, call.Pair{Old: "z", New: "a — b"}), verdict.Advise},
		{"marker kept within its pair", edits("/r/x.md", call.Pair{Old: "x", New: "y"}, call.Pair{Old: "keep — this", New: "keep — this\nnew"}), verdict.Silent},
		{"empty write", write("/r/x.md", ""), verdict.Silent},
		{"write ignores the file it replaces", write("/r/x.md", "a — b"), verdict.Advise},
	}
	match := MemoryMatcher([]string{"/home/u/.claude/projects/*/memory/", ".claude/agent-memory-local/"})
	opts := Options{Memory: func(p string) bool { return match(p, "/repo") }}
	for _, tc := range cases {
		if got := AIMarkers(tc.in, opts).Worst(); got != tc.want {
			t.Errorf("%s: want %v got %v", tc.name, tc.want, got)
		}
	}
}

func TestMemoryOption(t *testing.T) {
	opts := Options{Memory: func(string) bool { return true }}
	wall := write("/r/x.py", "# one long story line about the past\n# two\n# three\n# four\n# five\nx = 1\n")
	if got := Comments(wall, Options{}).Worst(); got != verdict.Deny {
		t.Fatalf("control: want deny got %v", got)
	}
	if got := Comments(wall, opts).Worst(); got != verdict.Silent {
		t.Errorf("a memory path is exempt from the comment check, got %v", got)
	}
	if got := AIMarkers(write("/r/x.md", "a — b"), opts).Worst(); got != verdict.Silent {
		t.Errorf("a memory path is exempt from the marker check, got %v", got)
	}
}

func TestMemoryMatcher(t *testing.T) {
	match := MemoryMatcher([]string{"/home/u/.claude/projects/*/memory/", "/home/u/.claude/agent-memory/", ".claude/agent-memory-local/"})
	cases := []struct {
		path, cwd string
		want      bool
	}{
		{"/home/u/.claude/projects/-x/memory/a.md", "", true},
		{"/home/u/.claude/projects/-x/memory", "", false},
		{"/home/u/.claude/projects/-x/y/memory/a.md", "", false},
		{"/home/u/.claude/agent-memory/log-forensics/n.md", "", true},
		{"/repo/.claude/agent-memory-local/a/n.md", "/repo", true},
		{"/repo/.claude/agent-memory-local/a/n.md", "/elsewhere", false},
		{"/repo/.claude/agent-memory-local/../../src/x.go", "/repo", false},
		{".claude/agent-memory-local/a/n.md", "/repo", false},
	}
	for _, c := range cases {
		if got := match(c.path, c.cwd); got != c.want {
			t.Errorf("%s (cwd %s): want %v got %v", c.path, c.cwd, c.want, got)
		}
	}
}

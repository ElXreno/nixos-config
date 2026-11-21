package replay

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffTreatsLegacyDigitsAsLevelNames(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy.tsv")
	named := filepath.Join(dir, "named.tsv")
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(legacy, "background\t2\tserver &\nshell-tool\t1\tcat notes.txt\ngit-destructive\task\tgit reset --hard\n")
	write(named, "background\tdeny\tserver &\nshell-tool\tadvise\tcat notes.txt\ngit-destructive\task\tgit reset --hard\n")
	var out bytes.Buffer
	if err := Diff(legacy, named, &out); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{"regressions: tripped before, silent now (0)", "newly caught (0)"} {
		if !strings.Contains(report, want) {
			t.Errorf("want %q in:\n%s", want, report)
		}
	}
	if changes := strings.SplitN(report, "== outcome changes ==\n", 2)[1]; changes != "" {
		t.Errorf("the same hits in digit and name form must not change outcomes, got:\n%s", changes)
	}
}

func TestDiffKeysOnTheHash(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	old := write("old.tsv", "pkill\tdeny\t111111111111\tpkill -f x...\npkill\tdeny\t222222222222\tpkill -f x...\n")
	newer := write("new.tsv", "pkill\tdeny\t111111111111\tpkill -f x...\n")
	var out bytes.Buffer
	if err := Diff(old, newer, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "regressions: tripped before, silent now (1)") {
		t.Errorf("two commands with the same truncated text are told apart by hash:\n%s", out.String())
	}
	legacy := write("legacy.tsv", "pkill\tdeny\tpkill -f x...\n")
	out.Reset()
	if err := Diff(legacy, newer, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "regressions: tripped before, silent now (0)") || !strings.Contains(out.String(), "newly caught (0)") {
		t.Errorf("a legacy file is matched on the text:\n%s", out.String())
	}
}

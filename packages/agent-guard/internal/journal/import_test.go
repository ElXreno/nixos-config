package journal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-guard/internal/harness"
)

var importNow = func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }

func opts() ImportOptions {
	return ImportOptions{Harness: harness.ClaudeCode, Now: importNow, Settle: 5 * time.Millisecond}
}

func legacyLine(i int, hook string) string {
	return `{"time":"2026-09-28T12:00:0` + string(rune('0'+i)) + `Z","session":"s","hook":"` + hook + `","tool":"Bash","level":"deny","checks":["pkill"],"input":"pkill -f x"}`
}

func write(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestImportPreservesLines(t *testing.T) {
	dir := t.TempDir()
	legacy, target := filepath.Join(dir, "verdicts.jsonl"), filepath.Join(dir, "state", "claude-code.jsonl")
	stamped := `{"time":"2026-09-28T12:00:05Z","harness":"pi","hook":"pre-bash","level":"silent"}`
	write(t, legacy, legacyLine(1, "pre-bash"), "not json", stamped, legacyLine(1, "pre-bash"))
	st, err := Import(target, legacy, opts())
	if err != nil {
		t.Fatal(err)
	}
	if st.Read != 4 || st.Appended != 2 || st.Duplicates != 1 || st.Undecodable != 1 {
		t.Errorf("counts: %+v", st)
	}
	got := lines(t, target)
	want := []string{`{"harness":"claude-code",` + legacyLine(1, "pre-bash")[1:], stamped}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines must be kept byte for byte apart from the harness:\n got %q\nwant %q", got, want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("the legacy journal must be moved away, %v", err)
	}
	if kept := lines(t, legacy+".imported-20261106T120000Z"); len(kept) != 4 {
		t.Errorf("the kept copy holds every original line, got %d", len(kept))
	}
	entries, skipped, err := Read(target)
	if err != nil || skipped != 0 || len(entries) != 2 || entries[0].Harness != harness.ClaudeCode || entries[0].Checks[0].Check != "pkill" {
		t.Errorf("the imported journal reads back: %+v %d %v", entries, skipped, err)
	}
}

func TestImportRerunAppendsOnlyNewLines(t *testing.T) {
	dir := t.TempDir()
	legacy, target := filepath.Join(dir, "verdicts.jsonl"), filepath.Join(dir, "claude-code.jsonl")
	write(t, legacy, legacyLine(1, "pre-bash"), legacyLine(2, "pre-bash"))
	if _, err := Import(target, legacy, opts()); err != nil {
		t.Fatal(err)
	}
	write(t, legacy, legacyLine(2, "pre-bash"), legacyLine(3, "pre-bash"))
	later := opts()
	later.Now = func() time.Time { return importNow().Add(time.Hour) }
	st, err := Import(target, legacy, later)
	if err != nil {
		t.Fatal(err)
	}
	if st.Appended != 1 || st.Duplicates != 1 || len(lines(t, target)) != 3 {
		t.Errorf("a recreated legacy file adds only its new lines: %+v, %d lines", st, len(lines(t, target)))
	}
}

func TestImportResumes(t *testing.T) {
	dir := t.TempDir()
	legacy, target := filepath.Join(dir, "verdicts.jsonl"), filepath.Join(dir, "claude-code.jsonl")
	write(t, legacy+".importing", legacyLine(1, "pre-bash"), legacyLine(2, "pre-bash"))
	write(t, target, `{"harness":"claude-code",`+legacyLine(1, "pre-bash")[1:])
	st, err := Import(target, legacy, opts())
	if err != nil {
		t.Fatal(err)
	}
	if st.Appended != 1 || st.Duplicates != 1 || len(lines(t, target)) != 2 {
		t.Errorf("a leftover .importing resumes without duplicates: %+v", st)
	}
	if _, err := Import(target, legacy, opts()); err == nil {
		t.Error("nothing left to import must be an error")
	}
}

func TestImportCatchesTheTail(t *testing.T) {
	dir := t.TempDir()
	legacy, target := filepath.Join(dir, "verdicts.jsonl"), filepath.Join(dir, "claude-code.jsonl")
	write(t, legacy, legacyLine(0, "pre-bash"))
	fh, err := os.OpenFile(legacy, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		defer func() { _ = fh.Close() }()
		for i := 1; i <= 9; i++ {
			time.Sleep(2 * time.Millisecond)
			_, _ = fh.WriteString(legacyLine(i, "pre-edit") + "\n")
		}
	})
	o := opts()
	o.Settle = 100 * time.Millisecond
	st, err := Import(target, legacy, o)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if st.Appended != 10 || len(lines(t, target)) != 10 {
		t.Errorf("a hook that opened the file before the rename is caught: %+v", st)
	}
}

func TestImportRefusesItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.jsonl")
	write(t, path, legacyLine(1, "pre-bash"))
	if _, err := Import(path, path, opts()); err == nil {
		t.Error("importing a journal into itself must be refused")
	}
}

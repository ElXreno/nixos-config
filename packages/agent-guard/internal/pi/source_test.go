package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-guard/internal/call"
	"agent-guard/internal/transcript"
)

func TestSource(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "--home-u-repo--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"session","version":3,"id":"s1","timestamp":"2026-11-06T09:00:00.000Z","cwd":"/home/u/repo"}`,
		`{"type":"message","id":"a","timestamp":"2026-11-06T09:01:00.000Z","message":{"role":"assistant","content":[` +
			`{"type":"text","text":"x"},` +
			`{"type":"toolCall","id":"t1","name":"edit","arguments":{"path":"notes.md","edits":[{"oldText":"a","newText":"b"}]}},` +
			`{"type":"toolCall","id":"t2","name":"process","arguments":{"action":"start","name":"n","command":"make run"}},` +
			`{"type":"toolCall","id":"t3","name":"process","arguments":{"action":"list"}},` +
			`{"type":"toolCall","id":"t4","name":"read","arguments":{"path":"x"}}]}}`,
		`not json`,
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-11-06T09-00-00_s1.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stray.jsonl"), []byte(lines[1]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	records, st, err := transcript.Walk(Source{Home: "/home/u"}, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 1 || st.Skipped != 1 || len(records) != 2 {
		t.Fatalf("want one session file, one skipped line and two records, got %+v %d", st, len(records))
	}
	e, p := records[0].Call, records[1].Call
	if e.Kind != call.Edit || e.Change.Path != "/home/u/repo/notes.md" || e.Change.Apply != call.Original || e.Session != "s1" || e.Tool != "edit" {
		t.Errorf("edit record: %+v", e)
	}
	if p.Kind != call.Shell || p.Command != "make run" || p.Cwd != "/home/u/repo" || p.Tool != "process" {
		t.Errorf("process record: %+v", p)
	}
}

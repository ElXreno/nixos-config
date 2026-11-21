package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"agent-guard/internal/checks"
	"agent-guard/internal/claudecode"
	"agent-guard/internal/edit"
	"agent-guard/internal/guard"
)

type toolUse struct {
	name  string
	input map[string]any
}

func writeTranscript(t *testing.T, path string, uses ...toolUse) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, u := range uses {
		rec := map[string]any{"message": map[string]any{"content": []any{
			map[string]any{"type": "tool_use", "name": u.name, "input": u.input},
		}}}
		line, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func engine() *guard.Engine {
	return &guard.Engine{Checks: &checks.Settings{}, Edit: edit.Options{Previous: edit.Missing}, AssumeNew: true}
}

func TestEditsReadsSubagentTranscripts(t *testing.T) {
	dir := t.TempDir()
	wall := "# first line of a story\n# second line of it\n# third line of it\nx = 1\n"
	edited := "# first line of a story\n# second line of it\n# third line of it\ny = 2"
	top := toolUse{"Write", map[string]any{"file_path": "/r/top.py", "content": wall}}
	sub := toolUse{"Write", map[string]any{"file_path": "/r/sub.py", "content": wall}}
	ed := toolUse{"Edit", map[string]any{"file_path": "/r/missing.py", "old_string": "y = 1", "new_string": edited}}
	marker := toolUse{"Write", map[string]any{"file_path": "/r/notes.md", "content": "a \u2014 b\n"}}
	flow := toolUse{"Write", map[string]any{"file_path": "/r/flow.py", "content": wall}}
	writeTranscript(t, filepath.Join(dir, "p", "s1.jsonl"), top)
	writeTranscript(t, filepath.Join(dir, "p", "s1", "subagents", "agent-a.jsonl"), sub, top, ed, marker)
	writeTranscript(t, filepath.Join(dir, "p", "s1", "subagents", "workflows", "wf_x", "agent-b.jsonl"), flow)
	writeTranscript(t, filepath.Join(dir, "p", "s1", "subagents", "journal.jsonl"), toolUse{"Write", map[string]any{"file_path": "/r/ignored.py", "content": wall}})
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "p", "s1", "subagents", "agent-gone.jsonl")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	st, err := Edits(claudecode.Source{}, dir, false, engine(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Edits != 5 || st.Hits != 5 || st.Verdicts["deny comments"] != 4 || st.Verdicts["advise ai-markers"] != 1 {
		t.Errorf("want 5 edits with 4 comment denies and 1 marker advice, got %+v\n%s", st, out.String())
	}
	if st.Files != 4 || st.Unreadable != 1 {
		t.Errorf("want 4 transcripts with 1 unreadable, got %+v", st.Stats)
	}
	if strings.Contains(out.String(), "ignored.py") {
		t.Errorf("journal.jsonl is not a transcript:\n%s", out.String())
	}
	for _, w := range []string{
		"/r/top.py\t",
		"\tcomments\tWrite\ttop\t" + strconv.Itoa(len(wall)),
		"\tcomments\tWrite\tsub\t",
		"\tcomments\tEdit\tsub\t" + strconv.Itoa(len(edited)),
		"\tai-markers\tWrite\tsub\t",
		"/r/flow.py\t",
		"\tlocation\tWrite\t",
	} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("rows must contain %q:\n%s", w, out.String())
		}
	}

	var again strings.Builder
	if _, err := Edits(claudecode.Source{}, dir, false, engine(), &again); err != nil || again.String() != out.String() {
		t.Errorf("two runs must print the same rows")
	}

	out.Reset()
	st, err = Edits(claudecode.Source{}, dir, true, engine(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Edits != 1 || st.Hits != 1 {
		t.Errorf("-top-only must skip subagent transcripts, got %+v", st)
	}

	link := filepath.Join(t.TempDir(), "projects")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if st, err = Edits(claudecode.Source{}, link, false, engine(), &out); err != nil || st.Edits != 5 {
		t.Errorf("a symlinked projects dir must be walked too, got %+v %v", st, err)
	}
}

func TestCorpus(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, filepath.Join(dir, "p", "s1.jsonl"), toolUse{"Bash", map[string]any{"command": "ls"}}, toolUse{"Bash", map[string]any{"command": "ls"}})
	writeTranscript(t, filepath.Join(dir, "p", "s1", "subagents", "agent-a.jsonl"), toolUse{"Bash", map[string]any{"command": "make"}}, toolUse{"Read", map[string]any{"file_path": "/x"}})
	var out strings.Builder
	n, _, err := Corpus(claudecode.Source{}, dir, false, &out)
	if err != nil || n != 2 || out.String() != "ls\x00make\x00" {
		t.Errorf("want ls and make once each, got %d %q %v", n, out.String(), err)
	}
	out.Reset()
	if n, _, _ := Corpus(claudecode.Source{}, dir, true, &out); n != 1 {
		t.Errorf("-top-only keeps the top transcripts, got %d", n)
	}
}

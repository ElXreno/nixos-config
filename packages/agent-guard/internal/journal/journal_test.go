package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-guard/internal/call"
	"agent-guard/internal/harness"
	"agent-guard/internal/verdict"
)

func TestAppendAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "verdicts.jsonl")
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	Append(path, Entry{Time: now, Harness: harness.Pi, Hook: PreBash, Kind: call.Shell, Level: "deny", Checks: []Fired{{Check: "shell-tool", Level: verdict.Deny}}, Input: "find ."})
	Append(path, Entry{Time: now, Hook: PreEdit, Level: "silent", Input: "/r/x.go"})
	Append("", Entry{Time: now, Hook: PreBash, Level: "deny"})
	got, skipped, err := Read(path)
	if err != nil || skipped != 0 {
		t.Fatal(err, skipped)
	}
	if len(got) != 2 || got[0].Checks[0] != (Fired{Check: "shell-tool", Level: verdict.Deny}) || got[0].Kind != call.Shell || got[0].Harness != harness.Pi || got[1].Input != "/r/x.go" {
		t.Fatalf("round trip lost data: %+v", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"kind":"shell","level":"deny","checks":[{"check":"shell-tool","level":"deny"}]`) || strings.Contains(strings.Split(string(raw), "\n")[1], `"kind"`) {
		t.Errorf("unexpected encoding:\n%s", raw)
	}
}

func TestReadMixesFormats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.jsonl")
	lines := []string{
		`{"time":"2026-09-28T12:00:00Z","hook":"pre-bash","tool":"Bash","level":"deny","checks":["git-scope","sleep"],"input":"git add -A && sleep 45"}`,
		`{"time":"2026-09-28T12:00:00Z","hook":"pre-bash","tool":"Bash","level":"rewrite","checks":["rg-replace"],"input":"rg -rn x"}`,
		`{"time":"2026-09-28T12:00:00Z","hook":"pre-bash","tool":"Bash","level":"2","checks":["pkill"],"input":"pkill -f x"}`,
		`{"time":"2026-09-28T12:00:00Z","harness":"claude-code","hook":"pre-bash","tool":"Bash","kind":"shell","level":"deny","checks":[{"check":"git-scope","level":"deny"},{"check":"sleep","level":"advise"}],"input":"git add -A && sleep 45"}`,
		`{"time":"2026-09-28T12:00:00Z","hook":"pre-bash","level":"deny","input":"` + strings.Repeat("x", maxLine) + `"}`,
		`not json`,
		`{"time":"2026-09-28T12:00:00Z","hook":"pre-edit","level":"silent","input":"/r/last-without-newline"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 2 || len(got) != 5 {
		t.Fatalf("want 5 entries and 2 skipped lines, got %d and %d", len(got), skipped)
	}
	if got[4].Input != "/r/last-without-newline" {
		t.Errorf("the last line without a newline was lost: %+v", got[4])
	}
	var b strings.Builder
	if err := Summarize(got, time.Time{}, "").Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"2  deny git-scope",
		"1  deny sleep",
		"1  advise sleep",
		"1  rewrite rg-replace",
		"1  deny pkill",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("summary misses %q:\n%s", want, out)
		}
	}
}

func TestSummarize(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	ask := []Fired{{Check: "git-destructive", Level: verdict.Ask}, {Check: "sleep", Level: verdict.Advise}}
	entries := []Entry{
		{Time: now, Session: "s", Hook: PreBash, ToolUseID: "a", Tool: "Bash", Level: "ask", Checks: ask, Input: "git reset --hard"},
		{Time: now, Session: "s", Hook: PermissionRequest, Tool: "Bash", Level: "dialog", Input: "git reset --hard"},
		{Time: now, Session: "other", Hook: PermissionRequest, Tool: "Bash", Level: "dialog", Input: "git reset --hard"},
		{Time: now, Session: "p", Harness: harness.Pi, Hook: PreBash, ToolUseID: "b", Tool: "bash", Level: "ask", Checks: ask[:1], Input: "git stash"},
		{Time: now, Session: "p", Harness: harness.Pi, Hook: PermissionRequest, ToolUseID: "b", Tool: "bash", Level: "dialog", Input: "git stash"},
		{Time: now, Session: "p", Harness: harness.Pi, Hook: PermissionRequest, Tool: "edit", Level: "dialog", Input: "/r/x"},
		{Time: now, Session: "s", Hook: PermissionRequest, Tool: "WebFetch", Level: "dialog", Input: "https://x"},
		{Time: now, Session: "s", Hook: PermissionRequest, Tool: "Bash", Level: "deny", Input: "rm x"},
		{Time: now, Hook: PreBash, Level: "deny", Checks: []Fired{{Check: "shell-tool", Level: verdict.Deny}}},
		{Time: now, Hook: PreBash, Level: "silent"},
		{Time: old, Hook: PreBash, Level: "deny", Checks: []Fired{{Check: "pkill", Level: verdict.Deny}}},
	}
	var b strings.Builder
	if err := Summarize(entries, now.Add(-24*time.Hour), harness.ClaudeCode).Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"2  guard ask: git-destructive",
		"1  Claude Code itself: WebFetch",
		"1  Claude Code itself: Bash",
		"1  pi itself: edit",
		"1  auto-denied: Bash",
		"1  deny shell-tool",
		"2  ask git-destructive",
		"1  advise sleep",
		"4  pre-bash",
		"6  permission-request",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("summary misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "pkill") {
		t.Errorf("an entry older than the window leaked in:\n%s", out)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLedger(t *testing.T, budget int) ledger {
	t.Helper()
	clock := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	return ledger{dir: filepath.Join(t.TempDir(), "ledger"), budget: budget, now: func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}}
}

func run(t *testing.T, l ledger, in hookInput) string {
	t.Helper()
	var out strings.Builder
	if err := l.handle(in, &out); err != nil {
		t.Fatalf("%s: %v", in.HookEventName, err)
	}
	return out.String()
}

func compact(l ledger, t *testing.T, session, summary string) {
	t.Helper()
	if out := run(t, l, hookInput{SessionID: session, HookEventName: "PostCompact", Trigger: "auto", CompactSummary: summary}); out != "" {
		t.Fatalf("PostCompact printed %q", out)
	}
}

func TestPreCompactPrintsInstructions(t *testing.T) {
	out := run(t, testLedger(t, 1000), hookInput{SessionID: "s1", HookEventName: "PreCompact", Trigger: "auto"})
	if !strings.Contains(out, `"Ruled out"`) || !strings.Contains(out, `"Established facts"`) {
		t.Fatalf("instructions missing sections: %q", out)
	}
}

func TestInjectsEarlierSummariesOldestFirst(t *testing.T) {
	l := testLedger(t, 1000)
	compact(l, t, "s1", "first summary")
	compact(l, t, "s1", "second summary")
	compact(l, t, "other", "foreign summary")
	out := run(t, l, hookInput{SessionID: "s1", HookEventName: "SessionStart", Source: "compact"})
	first, second := strings.Index(out, "first summary"), strings.Index(out, "second summary")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("want both summaries oldest first, got %q", out)
	}
	if strings.Contains(out, "foreign summary") {
		t.Fatalf("leaked another session: %q", out)
	}
	if !strings.Contains(out, "(2 of 2)") || !strings.Contains(out, "## Compaction 2 of 2 (2026-11-26 10:02 UTC, auto)") {
		t.Fatalf("missing counts or headings: %q", out)
	}
}

func TestBudgetKeepsNewest(t *testing.T) {
	l := testLedger(t, 25)
	compact(l, t, "s1", strings.Repeat("a", 20))
	compact(l, t, "s1", strings.Repeat("b", 20))
	compact(l, t, "s1", strings.Repeat("c", 20))
	out := run(t, l, hookInput{SessionID: "s1", HookEventName: "SessionStart", Source: "compact"})
	if strings.Contains(out, "aaaa") || strings.Contains(out, "bbbb") || !strings.Contains(out, strings.Repeat("c", 20)) {
		t.Fatalf("budget should keep only the newest: %q", out)
	}
	if !strings.Contains(out, "(1 of 3)") {
		t.Fatalf("want 1 of 3: %q", out)
	}
}

func TestOversizedNewestIsTruncated(t *testing.T) {
	l := testLedger(t, 10)
	compact(l, t, "s1", strings.Repeat("x", 50))
	out := run(t, l, hookInput{SessionID: "s1", HookEventName: "SessionStart", Source: "compact"})
	if strings.Contains(out, strings.Repeat("x", 11)) || !strings.Contains(out, strings.Repeat("x", 10)) {
		t.Fatalf("want the newest cut to the budget: %q", out)
	}
}

func TestQuietCases(t *testing.T) {
	l := testLedger(t, 1000)
	if out := run(t, l, hookInput{SessionID: "s1", HookEventName: "SessionStart", Source: "compact"}); out != "" {
		t.Fatalf("empty ledger printed %q", out)
	}
	compact(l, t, "s1", "kept")
	for _, source := range []string{"startup", "resume", "clear"} {
		if out := run(t, l, hookInput{SessionID: "s1", HookEventName: "SessionStart", Source: source}); out != "" {
			t.Fatalf("source %s printed %q", source, out)
		}
	}
	compact(l, t, "s1", "   ")
	entries, err := l.load("s1")
	if err != nil || len(entries) != 1 {
		t.Fatalf("blank summary must not be recorded: %v %d", err, len(entries))
	}
}

func TestRejectsBadSessionAndKeepsPrivateFiles(t *testing.T) {
	l := testLedger(t, 1000)
	var out strings.Builder
	if err := l.handle(hookInput{SessionID: "../escape", HookEventName: "PostCompact", CompactSummary: "x"}, &out); err == nil {
		t.Fatal("path traversal accepted")
	}
	compact(l, t, "s1", "private")
	for path, want := range map[string]os.FileMode{l.dir: 0o700, filepath.Join(l.dir, "s1.jsonl"): 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: mode %v, want %v (%v)", path, info.Mode().Perm(), want, err)
		}
	}
}

func TestSkipsCorruptLines(t *testing.T) {
	l := testLedger(t, 1000)
	compact(l, t, "s1", "good")
	f, err := os.OpenFile(filepath.Join(l.dir, "s1.jsonl"), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{not json\n")
	_ = f.Close()
	compact(l, t, "s1", "after")
	entries, err := l.load("s1")
	if err != nil || len(entries) != 2 {
		t.Fatalf("want 2 entries around a corrupt line: %v %d", err, len(entries))
	}
}

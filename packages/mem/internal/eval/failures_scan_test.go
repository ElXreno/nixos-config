package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func bashUse(id, cmd, ts string) map[string]any {
	return map[string]any{"type": "assistant", "timestamp": ts, "sessionId": "s1", "cwd": "/w",
		"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": cmd}}}}}
}

func otherUse(id, name, ts string) map[string]any {
	return map[string]any{"type": "assistant", "timestamp": ts, "sessionId": "s1", "cwd": "/w",
		"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{"file_path": "/x"}}}}}
}

func bashResult(id string, content any, isErr bool, ts string) map[string]any {
	return map[string]any{"type": "user", "timestamp": ts, "sessionId": "s1", "cwd": "/w", "toolUseResult": "Error: x",
		"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": isErr, "content": content}}}}
}

func withAgent(m map[string]any, agent string) map[string]any {
	m["agentId"] = agent
	return m
}

func jsonlOf(t *testing.T, entries ...map[string]any) string {
	t.Helper()
	var b strings.Builder
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestScanFailures(t *testing.T) {
	long := "Exit code 1\n" + strings.Repeat("x", 5<<20) + "\nthe end"
	cases := []struct {
		name    string
		entries []map[string]any
		want    []string
		agent   string
	}{
		{"string content", []map[string]any{bashUse("t1", "make", "2026-11-08T10:00:00.250Z"), bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:01.500Z")},
			[]string{"make|Exit code 2\nmake: no rule|2026-11-08T10:00:01.5Z"}, ""},
		{"block content", []map[string]any{bashUse("t1", "ls /x", "2026-11-08T10:00:00Z"), bashResult("t1", []any{map[string]any{"type": "text", "text": "Exit code 2\nls: cannot access"}}, true, "2026-11-08T10:00:02Z")},
			[]string{"ls /x|Exit code 2\nls: cannot access|2026-11-08T10:00:02Z"}, ""},
		{"a failed Read is not a Bash failure", []map[string]any{otherUse("t1", "Read", "2026-11-08T10:00:00Z"), bashResult("t1", "Exit code 1\nno such file", true, "2026-11-08T10:00:01Z")}, nil, ""},
		{"an error without Exit code", []map[string]any{bashUse("t1", "git push", "2026-11-08T10:00:00Z"), bashResult("t1", "PreToolUse:Bash hook error: Blocked", true, "2026-11-08T10:00:01Z")}, nil, ""},
		{"not an error", []map[string]any{bashUse("t1", "false", "2026-11-08T10:00:00Z"), bashResult("t1", "Exit code 1\nlooks like one", false, "2026-11-08T10:00:01Z")}, nil, ""},
		{"a result without its call", []map[string]any{bashResult("t9", "Exit code 1\norphan", true, "2026-11-08T10:00:01Z")}, nil, ""},
		{"a subagent line", []map[string]any{withAgent(bashUse("t1", "make", "2026-11-08T10:00:00Z"), "a7"), withAgent(bashResult("t1", "Exit code 2\nfail", true, "2026-11-08T10:00:01Z"), "a7")},
			[]string{"make|Exit code 2\nfail|2026-11-08T10:00:01Z"}, "a7"},
		{"a very long line", []map[string]any{bashUse("t1", "cat big", "2026-11-08T10:00:00Z"), bashResult("t1", long, true, "2026-11-08T10:00:01Z")},
			[]string{"cat big|" + long + "|2026-11-08T10:00:01Z"}, ""},
	}
	for _, c := range cases {
		sc, err := scanFailures(strings.NewReader(jsonlOf(t, c.entries...)+"not json\n"), "f.jsonl")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []string
		for _, ev := range sc.events {
			got = append(got, ev.Command+"|"+ev.Error+"|"+ev.Time.UTC().Format(time.RFC3339Nano))
			if ev.Agent != c.agent || ev.Session != "s1" || ev.Cwd != "/w" || ev.File != "f.jsonl" {
				t.Errorf("%s: event %+v", c.name, ev.Agent)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %d events, want %d", c.name, len(got), len(c.want))
		}
	}
}

func TestMemGetKeys(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{"mem get a b c", []string{"a", "b", "c"}},
		{"mem get --body a b && ls", []string{"a", "b"}},
		{"mem get a | sed -n 1,5p", []string{"a"}},
		{"mem get --json --history k1", []string{"k1"}},
		{"somemem get x", nil},
		{"mem search get x", nil},
		{"echo get mem", nil},
		{"mem get a; mem get b", []string{"a", "b"}},
		{"mem get a\nmem get b", []string{"a", "b"}},
		{"mem get a || true", []string{"a"}},
		{"mem get a & wait", []string{"a"}},
		{"mem get a 2>/dev/null b", []string{"a"}},
		{"mem get a > /tmp/out b", []string{"a"}},
		{"mem get a <<< x", []string{"a"}},
		{"echo $(mem get zfs-arc-ram) done", []string{"zfs-arc-ram"}},
		{"mem get 'quoted-key' \"dq.key\" a\\ b", []string{"quoted-key", "dq.key"}},
		{"mem get Upper_Case ok_key $VAR", []string{"ok_key"}},
		{"mem get bt-journal#1 topic:x", []string{"bt-journal#1", "topic:x"}},
		{"cat <<'E'\nmem get inside\nE\nmem get after", []string{"after"}},
		{"cat <<-EOF | wc -l\n\tmem get inside\n\tEOF\nmem get after", []string{"after"}},
		{"# mem get commented\nmem get real", []string{"real"}},
		{"/run/current-system/sw/bin/mem get abs", []string{"abs"}},
		{"mem --db /x/mem.db get k", []string{"k"}},
		{"mem get a a b a", []string{"a", "b"}},
		{"mem get \\\n  continued", []string{"continued"}},
		{"rg 'mem get x' file", nil},
	}
	for _, c := range cases {
		got := memGetKeys(c.cmd)
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: %q, want %q", c.cmd, got, c.want)
		}
	}
}

func TestLookupHorizon(t *testing.T) {
	tr := jsonlOf(t,
		bashUse("t0", "mem get before", "2026-11-08T10:00:00Z"),
		bashUse("t1", "make", "2026-11-08T10:00:01Z"),
		bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:02Z"),
		otherUse("r1", "Read", "2026-11-08T10:00:03Z"),
		bashUse("t2", "ls", "2026-11-08T10:00:04Z"),
		bashUse("t3", "mem get second", "2026-11-08T10:00:05Z"),
		bashUse("t4", "mem get third second", "2026-11-08T10:00:06Z"),
	)
	sc, err := scanFailures(strings.NewReader(tr), "f")
	if err != nil || len(sc.events) != 1 {
		t.Fatalf("%v %d", err, len(sc.events))
	}
	cases := []struct {
		horizon int
		want    []string
	}{
		{1, nil},
		{2, []string{"second"}},
		{3, []string{"second", "third"}},
		{15, []string{"second", "third"}},
	}
	for _, c := range cases {
		if got := sc.lookups(sc.events[0], c.horizon); !slices.Equal(got, c.want) {
			t.Errorf("horizon %d: %q, want %q", c.horizon, got, c.want)
		}
	}
}

func inMessage(m map[string]any, id string) map[string]any {
	m["message"].(map[string]any)["id"] = id
	return m
}

func TestLookupsSkipCallsOfTheFailingMessage(t *testing.T) {
	cases := []struct {
		name    string
		horizon int
		entries []map[string]any
		want    []string
	}{
		{"sibling issued before the result", 15, []map[string]any{
			inMessage(bashUse("t1", "make", "2026-11-08T10:00:00Z"), "msg_a"),
			inMessage(bashUse("t2", "mem get unrelated-key", "2026-11-08T10:00:00Z"), "msg_a"),
			bashResult("t2", "ok", false, "2026-11-08T10:00:01Z"),
			bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:02Z"),
			inMessage(bashUse("t3", "mem get real", "2026-11-08T10:00:03Z"), "msg_b"),
		}, []string{"real"}},
		{"sibling streamed after the result", 15, []map[string]any{
			inMessage(bashUse("t1", "make", "2026-11-08T10:00:00Z"), "msg_a"),
			bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:02Z"),
			inMessage(bashUse("t2", "mem get unrelated-key", "2026-11-08T10:00:03Z"), "msg_a"),
			bashResult("t2", "ok", false, "2026-11-08T10:00:04Z"),
			inMessage(bashUse("t3", "mem get real", "2026-11-08T10:00:05Z"), "msg_b"),
		}, []string{"real"}},
		{"siblings do not use up the horizon", 1, []map[string]any{
			inMessage(bashUse("t1", "make", "2026-11-08T10:00:00Z"), "msg_a"),
			inMessage(bashUse("t2", "ls", "2026-11-08T10:00:00Z"), "msg_a"),
			inMessage(bashUse("t3", "pwd", "2026-11-08T10:00:00Z"), "msg_a"),
			bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:02Z"),
			inMessage(bashUse("t4", "mem get real", "2026-11-08T10:00:03Z"), "msg_b"),
			inMessage(bashUse("t5", "mem get beyond", "2026-11-08T10:00:04Z"), "msg_c"),
		}, []string{"real"}},
		{"calls without a message id count as before", 1, []map[string]any{
			bashUse("t1", "make", "2026-11-08T10:00:00Z"),
			bashUse("t2", "mem get sibling", "2026-11-08T10:00:00Z"),
			bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:02Z"),
			bashUse("t3", "mem get beyond", "2026-11-08T10:00:03Z"),
		}, []string{"sibling"}},
	}
	for _, c := range cases {
		sc, err := scanFailures(strings.NewReader(jsonlOf(t, c.entries...)), "f")
		if err != nil || len(sc.events) != 1 {
			t.Fatalf("%s: %v %d", c.name, err, len(sc.events))
		}
		if got := sc.lookups(sc.events[0], c.horizon); !slices.Equal(got, c.want) {
			t.Errorf("%s: lookups %q, want %q", c.name, got, c.want)
		}
	}
}

func TestScanAllSinceAndGlob(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "projects", "p", "s1.jsonl")
	sub := filepath.Join(root, "projects", "p", "s1", "subagents", "agent-a1.jsonl")
	deep := filepath.Join(root, "projects", "p", "s1", "subagents", "workflows", "w", "agent-a2.jsonl")
	for path, body := range map[string]string{
		main: jsonlOf(t, bashUse("t1", "early", "2026-11-06T22:00:00Z"), bashResult("t1", "Exit code 1\nold", true, "2026-11-06T22:00:01Z"),
			bashUse("t2", "late", "2026-11-08T09:00:00Z"), bashResult("t2", "Exit code 1\nnew", true, "2026-11-08T09:00:01Z")),
		sub:  jsonlOf(t, bashUse("t1", "sub", "2026-11-07T23:00:00Z"), bashResult("t1", "Exit code 1\nsub", true, "2026-11-07T23:00:01Z")),
		deep: jsonlOf(t, bashUse("t1", "deep", "2026-11-07T23:30:00Z"), bashResult("t1", "Exit code 1\ndeep", true, "2026-11-07T23:30:01Z")),
		filepath.Join(root, "projects", "p", "notes.txt"): "x",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	globs := []struct {
		glob string
		want []string
	}{
		{filepath.Join(root, "projects", "**", "*.jsonl"), []string{main, sub, deep}},
		{filepath.Join(root, "projects", "*", "*.jsonl"), []string{main}},
		{filepath.Join(root, "projects", "p", "**", "agent-*.jsonl"), []string{sub, deep}},
		{"~/projects/**/s1.jsonl", []string{main}},
		{filepath.Join(root, "nothing", "**", "*.jsonl"), nil},
	}
	for _, g := range globs {
		got, err := globFiles(g.glob, root)
		if err != nil {
			t.Fatal(err)
		}
		want := slices.Clone(g.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", g.glob, got, want)
		}
	}
	since := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	files, _ := globFiles(filepath.Join(root, "projects", "**", "*.jsonl"), "")
	got, err := scanAll(files, since, 15, 2)
	if err != nil {
		t.Fatal(err)
	}
	var cmds []string
	for _, c := range got {
		cmds = append(cmds, c.Command)
	}
	if !slices.Equal(cmds, []string{"sub", "deep", "late"}) {
		t.Errorf("since and time order: %q", cmds)
	}
}

package hook

import (
	"strings"
	"testing"
)

const arcFailure = "Exit code 1\ncannot import 'tank': out of memory, the zfs arc did not shrink in time"

func (sb *sandbox) failure(t *testing.T, session, command, errText string, interrupt bool) string {
	t.Helper()
	return jsonLine(t, map[string]any{"session_id": session, "cwd": sb.root + "/repos/nixos-config", "hook_event_name": "PostToolUseFailure",
		"tool_name": "Bash", "tool_input": map[string]any{"command": command}, "tool_use_id": "toolu_1", "error": errText, "is_interrupt": interrupt})
}

func failureArgs(extra ...string) []string {
	return append(append([]string{ToolFailure}, vecFlags...), extra...)
}

func TestToolFailureGetsPointersOnly(t *testing.T) {
	sb := agentSandbox(t)
	in := sb.failure(t, "f1", "zpool import tank", arcFailure, false)
	sb.check(t, spec{name: "a failed command", event: ToolFailure, stdin: in, args: vecFlags,
		has:   []string{failureHeader + "- `zfs-arc-ram` [fact, as of 2026-10-01]: ZFS ARC size after boot"},
		lacks: []string{"grows to half of RAM", "records related to this prompt"},
		log:   []string{"post-tool-use-failure session=f1 inject", "embed=ok", "tool Bash pointers zfs-arc-ram"}})
	turns := sb.turns(t)
	last := turns[len(turns)-1]
	if last.Tool != "ToolFailure:Bash" || last.Status != "inject" || len(last.Pointers) != 1 || last.Pointers[0].Key != "zfs-arc-ram" || len(last.Strong) != 0 || last.Input != "zpool import tank" {
		t.Errorf("turn %+v", last)
	}

	sb.check(t, spec{name: "the same failure again", event: ToolFailure, stdin: in, args: vecFlags, empty: true, log: []string{"session=f1 skip-repeat"}})
	sb.check(t, spec{name: "another failure that points at the same record", event: ToolFailure,
		stdin: sb.failure(t, "f1", "zfs list", "Exit code 2\nthe zfs arc is at its limit and the pool refuses new writes", false),
		args:  vecFlags, empty: true, log: []string{"session=f1 none"}})

	got := sb.context(t, promptInput(t, sb, "f1", "Why is the ZFS ARC so large?"), append([]string{UserPromptSubmit}, vecFlags...)...)
	if !strings.Contains(got, "`zfs-arc-ram` [fact, as of 2026-10-01]: ZFS ARC size after boot. The ZFS ARC grows to half of RAM") {
		t.Errorf("a pointer from a failure must not keep the record out of a later prompt:\n%s", got)
	}

	sb.check(t, spec{name: "an interrupt", event: ToolFailure, stdin: sb.failure(t, "f2", "zpool import tank", arcFailure, true),
		args: vecFlags, empty: true, log: []string{"session=f2 skip-interrupt"}})
	for _, short := range []string{"Exit code 1", "Exit code 127\nnot found", "", "   "} {
		sb.check(t, spec{name: "an error without text", event: ToolFailure, stdin: sb.failure(t, "f2", "zpool import tank", short, false),
			args: vecFlags, empty: true, log: []string{"session=f2 skip-short"}})
	}
}

func TestToolFailureTakesThreePointers(t *testing.T) {
	sb := agentSandbox(t)
	got := sb.context(t, sb.failure(t, "f3", "frobnicate --all", "Exit code 1\nthe frobnicator module stopped at line 42 with an unknown state", false), failureArgs()...)
	if !strings.HasPrefix(got, failureHeader) || strings.Count(got, "\n- `") != failureTake {
		t.Errorf("want the header and %d pointer lines:\n%s", failureTake, got)
	}
}

func TestToolFailureAsOf(t *testing.T) {
	sb := agentSandbox(t)
	in := func(session string) string { return sb.failure(t, session, "zpool import tank", arcFailure, false) }
	if got := sb.context(t, in("f4"), failureArgs("--as-of", "2026-09-01T00:00:00Z")...); got != "" {
		t.Errorf("records written after the replayed moment are used: %q", got)
	}
	if got := sb.context(t, in("f5"), failureArgs("--as-of", "2026-10-02T00:00:00Z")...); !strings.Contains(got, "`zfs-arc-ram`") {
		t.Errorf("records that existed at the replayed moment are lost: %q", got)
	}
	code, out, _ := sb.run(in("f6"), failureArgs("--as-of", "yesterday")...)
	if code != 0 || out != "" || !strings.Contains(sb.lastLog(t), "bad flags") {
		t.Errorf("a bad --as-of: %d %q, log %q", code, out, sb.lastLog(t))
	}
	prompt := promptInput(t, sb, "f7", "Why is the ZFS ARC so large?")
	if got := sb.context(t, prompt, append([]string{UserPromptSubmit, "--as-of", "2026-09-01T00:00:00Z"}, vecFlags...)...); strings.Contains(got, "zfs-arc-ram") {
		t.Errorf("--as-of on the prompt hook: %q", got)
	}
	if got := sb.context(t, prompt, append([]string{UserPromptSubmit, "--as-of", "2026-10-02T00:00:00Z"}, vecFlags...)...); !strings.Contains(got, "`zfs-arc-ram` [fact") {
		t.Errorf("--as-of after the records were written: %q", got)
	}
}

func TestFailureQuery(t *testing.T) {
	long := strings.Repeat("x", 400)
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"exit line dropped", Input{ToolInput: []byte(`{"command":"make"}`), Error: "Exit code 2\nmake: *** No rule to make target 'all'.  Stop."}, "make\nmake: *** No rule to make target 'all'.  Stop."},
		{"no exit line", Input{ToolInput: []byte(`{"file_path":"/x/y.go"}`), Error: "String to replace not found in file, check the whitespace"}, "/x/y.go\nString to replace not found in file, check the whitespace"},
		{"too short", Input{ToolInput: []byte(`{"command":"make"}`), Error: "Exit code 2\nfailed"}, ""},
		{"command clipped", Input{ToolInput: []byte(`{"command":"` + long + `"}`), Error: "Exit code 1\nsomething long enough to count as text"}, long[:failureCommandRunes] + "\nsomething long enough to count as text"},
	}
	for _, c := range cases {
		if got := failureQuery(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	tail := failureQuery(Input{ToolInput: []byte(`{"command":"make"}`), Error: "Exit code 1\n" + strings.Repeat("noise ", 400) + "the last line matters"})
	if !strings.HasSuffix(tail, "the last line matters") || len(tail) > failureErrorRunes+10 {
		t.Errorf("the tail of a long error: %d chars, %q", len(tail), tail[len(tail)-40:])
	}
}

package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mem/internal/statefile"
)

func toolInputWith(t *testing.T, sb *sandbox, tool string, fields map[string]any, extra map[string]any) string {
	t.Helper()
	m := map[string]any{"session_id": "tc", "cwd": sb.root + "/repos/nixos-config", "hook_event_name": "PreToolUse", "tool_name": tool, "tool_input": fields}
	for k, v := range extra {
		m[k] = v
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestToolTurnCarriesContextInputAndTheRegexThatFired(t *testing.T) {
	sb := kurwaSandbox(t, "KURWA.angora-ide.ts.net")
	_, out, _ := sb.run(toolInput(t, sb, "tc", "", "git   push\torigin main"), PreToolUse)
	if out == "" {
		t.Fatal("no injection")
	}
	lines := sb.turns(t)
	if len(lines) != 1 {
		t.Fatalf("turns %+v", lines)
	}
	tl := lines[0]
	if tl.Event != statefile.EventTool || tl.Input != "git push origin main" || tl.Harness != "claude-code" || tl.Host != "kurwa" || tl.Project != "git.example/nixos-config" && tl.Project == "" {
		t.Errorf("context %+v", tl)
	}
	if !strings.HasSuffix(tl.Cwd, "/repos/nixos-config") && !strings.HasPrefix(tl.Cwd, "~/") {
		t.Errorf("cwd %q", tl.Cwd)
	}
	if len(tl.Fired) == 0 || len(tl.Fired) != len(tl.Keys) {
		t.Fatalf("fired %+v keys %v", tl.Fired, tl.Keys)
	}
	for _, f := range tl.Fired {
		if !slices.Contains(tl.Keys, f.Key) || f.Tool != "Bash" || f.Regex == "" {
			t.Errorf("fired %+v", f)
		}
	}
}

func TestToolInputPreviewPerTool(t *testing.T) {
	cases := []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"command": "ls  -la\n/tmp"}, "ls -la /tmp"},
		{map[string]any{"file_path": "/a/b.go", "old_string": "secret", "new_string": "x"}, "/a/b.go"},
		{map[string]any{"description": "Audit the ui", "prompt": "long prompt text", "subagent_type": "Explore"}, "long prompt text | Audit the ui"},
		{map[string]any{"file_path": "/a/b.go", "description": "edit it"}, "/a/b.go | edit it"},
		{map[string]any{"command": strings.Repeat("x", 400)}, strings.Repeat("x", statefile.InputRunes)},
		{map[string]any{"other": 1}, ""},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(c.fields)
		if got := toolPreview(Input{ToolInput: raw}); got != c.want {
			t.Errorf("%v = %q, want %q", c.fields, got, c.want)
		}
	}
}

func TestPromptAndSkippedTurnsCarryContext(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.run(promptInput(t, sb, "p1", pushPrompt), UserPromptSubmit, "--harness", "pi")
	sb.run(promptInput(t, sb, "p1", "<task-notification>done</task-notification>"), UserPromptSubmit)
	lines := sb.turns(t)
	if len(lines) != 2 {
		t.Fatalf("turns %+v", lines)
	}
	if lines[0].Harness != "pi" || lines[1].Harness != "claude-code" || lines[1].Status != "skipped" || lines[1].Cwd == "" {
		t.Errorf("turns %+v", lines)
	}
}

func TestSubagentTypeIsLogged(t *testing.T) {
	sb := kurwaSandbox(t, "KURWA.angora-ide.ts.net")
	sb.run(toolInputWith(t, sb, "Bash", map[string]any{"command": "git push"}, map[string]any{"agent_id": "a1", "agent_type": "Explore"}), PreToolUse)
	lines := sb.turns(t)
	if len(lines) != 1 || lines[0].Agent != "a1" || lines[0].AgentType != "Explore" {
		t.Fatalf("turns %+v", lines)
	}
}

func TestSessionStartWritesATurn(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	_, out, _ := sb.run(sessionStartInput(sb, "ss1"), SessionStart)
	lines := sb.turns(t)
	if len(lines) != 1 {
		t.Fatalf("turns %+v", lines)
	}
	tl := lines[0]
	if tl.Event != statefile.EventSession || tl.Session != "ss1" || tl.Status != "inject" || tl.Chars == 0 || tl.Harness != "claude-code" {
		t.Errorf("session turn %+v", tl)
	}
	if out == "" || len(tl.Keys) == 0 {
		t.Errorf("out %q keys %v", out, tl.Keys)
	}
	for _, k := range tl.Keys {
		if strings.HasPrefix(k, "~") {
			t.Errorf("marker %q leaked into keys", k)
		}
	}
}

func TestSessionStartTurnsCanBeSwitchedOff(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.run(sessionStartInput(sb, "ss2"), SessionStart, "--no-turns-log")
	if _, err := os.Stat(sb.turnsPath()); !os.IsNotExist(err) {
		t.Errorf("turns file written: %v", err)
	}
}

func TestErrorTurnsAreWritten(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	db := filepath.Join(sb.vars["XDG_DATA_HOME"], "agent-memory", "mem.db")
	if err := os.WriteFile(db, []byte("not a database, not a database, not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	sb.run(promptInput(t, sb, "er", pushPrompt), UserPromptSubmit)
	sb.run(toolInputWith(t, sb, "Edit", map[string]any{"file_path": "/x/y"}, nil), PreToolUse)
	lines := sb.turns(t)
	if len(lines) != 2 {
		t.Fatalf("turns %+v", lines)
	}
	if lines[0].Event != statefile.EventPrompt || lines[0].Status != "error" || lines[0].Prompt != pushPrompt && lines[0].Prompt == "" {
		t.Errorf("prompt error turn %+v", lines[0])
	}
	if lines[1].Event != statefile.EventTool || lines[1].Status != "error" || lines[1].Tool != "Edit" || lines[1].Input != "/x/y" {
		t.Errorf("tool error turn %+v", lines[1])
	}
}

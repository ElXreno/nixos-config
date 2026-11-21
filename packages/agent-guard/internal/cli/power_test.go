package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type rig struct {
	t                  *testing.T
	cfg, journal, root string
}

func newRig(t *testing.T, harness string, enable bool) *rig {
	t.Helper()
	dir := t.TempDir()
	journal := filepath.Join(dir, "state", harness+".jsonl")
	cfg := writeConfig(t, harness, map[string]any{
		"journal": journal,
		"power":   map[string]any{"enable": enable, "heavyCommands": []string{"nix build", "cargo test"}},
	})
	return &rig{t: t, cfg: cfg, journal: journal, root: filepath.Join(dir, "power_supply")}
}

func (r *rig) set(source string, capacity int) {
	r.t.Helper()
	if err := os.RemoveAll(r.root); err != nil {
		r.t.Fatal(err)
	}
	files := map[string]string{"AC/type": "Mains", "AC/online": "1"}
	switch source {
	case "ac":
		files["BAT0/type"], files["BAT0/status"] = "Battery", "Charging"
	case "battery":
		files["AC/online"] = "0"
		files["BAT0/type"], files["BAT0/status"] = "Battery", "Discharging"
	}
	if source != "desktop" {
		files["BAT0/capacity"] = strconv.Itoa(capacity)
	}
	for name, v := range files {
		p := filepath.Join(r.root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(v+"\n"), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *rig) run(stdin []byte, args ...string) result {
	r.t.Helper()
	env, stdout, stderr := testEnv(stdin)
	env.PowerRoot = r.root
	if len(args) == 0 {
		args = []string{"hook"}
	}
	code := Run(append([]string{"--config", r.cfg}, args...), env)
	j, _ := os.ReadFile(r.journal)
	return result{code, stdout.String(), stderr.String(), string(j)}
}

func ccInput(event, session string, extra map[string]any) []byte {
	in := map[string]any{"session_id": session, "cwd": "/home/test/repo", "hook_event_name": event}
	for k, v := range extra {
		in[k] = v
	}
	b, _ := json.Marshal(in)
	return b
}

func ccPost(session string) []byte {
	return ccInput("PostToolUse", session, map[string]any{"tool_name": "Read", "tool_input": map[string]string{"file_path": "/x"}, "tool_response": map[string]string{"content": "x"}, "tool_use_id": "toolu_post"})
}

func ccPrompt(session string) []byte {
	return ccInput("UserPromptSubmit", session, map[string]any{"prompt": "go on"})
}

func ccBash(session, command string) []byte {
	return ccInput("PreToolUse", session, map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": command}, "tool_use_id": "toolu_pre"})
}

func ccContext(event, text string) string {
	b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{"additionalContext": text, "hookEventName": event}})
	return strings.ReplaceAll(string(b), `\u003e`, ">") + "\n"
}

func heavyNote(capacity int) string {
	return "Power: on battery at " + strconv.Itoa(capacity) + "%, heavy build (`nix build`). If it can wait, `agent-guard power wait ac` run in the background exits once AC is back."
}

type step struct {
	name     string
	source   string
	capacity int
	stdin    []byte
	code     int
	stdout   string
}

func (r *rig) steps(steps []step) {
	r.t.Helper()
	for _, s := range steps {
		if s.source != "" {
			r.set(s.source, s.capacity)
		}
		got := r.run(s.stdin)
		if got.code != s.code || got.stdout != s.stdout {
			r.t.Errorf("%s: want exit %d stdout %q\ngot exit %d stdout %q stderr %q", s.name, s.code, s.stdout, got.code, got.stdout, got.stderr)
		}
	}
}

func TestPowerClaudeCode(t *testing.T) {
	r := newRig(t, "claude-code", true)
	r.steps([]step{
		{"first look on AC says nothing", "ac", 80, ccPost("s1"), 0, ""},
		{"a prompt on AC says nothing", "", 0, ccPrompt("s1"), 0, ""},
		{"unplugging is told once", "battery", 40, ccPost("s1"), 0, ccContext("PostToolUse", "Power: unplugged, now on battery at 40%.")},
		{"still on battery", "", 0, ccPost("s1"), 0, ""},
		{"the charge dropping is no change", "battery", 38, ccPrompt("s1"), 0, ""},
		{"a heavy build on battery", "", 0, ccBash("s1", "nix build --no-link .#x"), 0, ccContext("PreToolUse", heavyNote(38))},
		{"a light command on battery", "", 0, ccBash("s1", "ls"), 0, ""},
		{"plugging back in", "ac", 39, ccPrompt("s1"), 0, ccContext("UserPromptSubmit", "Power: back on AC, battery at 39%.")},
		{"a heavy build on AC", "", 0, ccBash("s1", "nix build --no-link .#x"), 0, ""},
		{"a new session on battery hears it once", "battery", 35, ccPrompt("s2"), 0, ccContext("UserPromptSubmit", "Power: on battery at 35%.")},
		{"then nothing", "", 0, ccPost("s2"), 0, ""},
		{"the heavy note counts as telling", "", 0, ccBash("s3", "cd x && sh -c 'cargo test'"), 0, ccContext("PreToolUse", strings.Replace(heavyNote(35), "nix build", "cargo test", 1))},
		{"so the next tool result stays quiet", "", 0, ccPost("s3"), 0, ""},
		{"a denied command carries no note", "", 0, ccBash("s4", "pkill -f x; nix build --no-link"), 2, ""},
		{"so the session still hears it", "", 0, ccPost("s4"), 0, ccContext("PostToolUse", "Power: on battery at 35%.")},
		{"a tool input that does not decode", "", 0, ccInput("PostToolUse", "s5", map[string]any{"tool_name": "mcp__x", "tool_input": "raw"}), 0, ccContext("PostToolUse", "Power: on battery at 35%.")},
		{"no session, no memory", "", 0, ccPost(""), 0, ""},
	})
	journal := r.run(ccPost("s1")).journal
	for _, want := range []string{
		`"session":"s1","tool_use_id":"toolu_post","hook":"post-tool","tool":"Read","level":"advise","checks":[{"check":"power","level":"advise"}],"input":"/x"}`,
		`"session":"s1","hook":"user-prompt","level":"advise","checks":[{"check":"power","level":"advise"}]}`,
		`"session":"s1","tool_use_id":"toolu_pre","hook":"pre-bash","tool":"Bash","kind":"shell","level":"advise","checks":[{"check":"power","level":"advise"}],"input":"nix build --no-link .#x"}`,
	} {
		if !strings.Contains(journal, want) {
			t.Errorf("journal lacks %s:\n%s", want, journal)
		}
	}
	if n := strings.Count(journal, `"hook":"post-tool"`); n != 4 {
		t.Errorf("only post-tool calls that said something are journaled, got %d:\n%s", n, journal)
	}
	if n := strings.Count(journal, `"hook":"user-prompt"`); n != 2 {
		t.Errorf("only prompts that said something are journaled, got %d", n)
	}
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(r.journal), "power"))
	if err != nil || len(entries) != 5 {
		t.Errorf("one state file per session beside the journal, got %d %v", len(entries), err)
	}
}

func TestPowerNoteRidesAlongOtherVerdicts(t *testing.T) {
	r := newRig(t, "claude-code", true)
	r.set("battery", 50)
	got := r.run(ccBash("s", "git reset --hard && nix build --no-link"))
	if got.code != 0 || !strings.Contains(got.stdout, `"permissionDecision":"ask"`) || !strings.Contains(got.stdout, `"additionalContext":"`+heavyNote(50)) {
		t.Errorf("an ask keeps its dialog and carries the note: %+v", got)
	}
	got = r.run(ccBash("s", "rg -rn foo . && nix build --no-link"))
	if got.code != 0 || !strings.Contains(got.stdout, `"updatedInput"`) || !strings.Contains(got.stdout, heavyNote(50)) {
		t.Errorf("a rewrite carries the note: %+v", got)
	}
	if !strings.Contains(got.journal, `"level":"rewrite","checks":[{"check":"rg-replace","level":"deny","fixed":true},{"check":"power","level":"advise"}]`) {
		t.Errorf("the journal carries both: %s", got.journal)
	}
}

func TestPowerSubagentsKeepTheirOwnState(t *testing.T) {
	r := newRig(t, "claude-code", true)
	sub := func(agent string) []byte {
		return ccInput("PostToolUse", "m", map[string]any{"agent_id": agent, "tool_name": "Read", "tool_input": map[string]string{"file_path": "/x"}, "tool_use_id": "toolu_sub"})
	}
	r.steps([]step{
		{"the main thread hears it at the prompt", "battery", 40, ccPrompt("m"), 0, ccContext("UserPromptSubmit", "Power: on battery at 40%.")},
		{"a subagent's first look on AC says nothing", "ac", 41, sub("a1"), 0, ""},
		{"so the main thread still hears the change", "", 0, ccPost("m"), 0, ccContext("PostToolUse", "Power: back on AC, battery at 41%.")},
		{"a new subagent on battery hears it", "battery", 39, sub("a2"), 0, ccContext("PostToolUse", "Power: on battery at 39%.")},
		{"and the main thread hears its own change", "", 0, ccPost("m"), 0, ccContext("PostToolUse", "Power: unplugged, now on battery at 39%.")},
		{"the first subagent too", "", 0, sub("a1"), 0, ccContext("PostToolUse", "Power: unplugged, now on battery at 39%.")},
	})
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(r.journal), "power"))
	if err != nil || len(entries) != 3 {
		t.Errorf("one state file for the main thread and one per subagent, got %d %v", len(entries), err)
	}
}

func TestPowerPiAskCarriesTheNote(t *testing.T) {
	r := newRig(t, "pi", true)
	r.set("battery", 50)
	bash := func(session string, ui bool) []byte {
		return piInput("tool_call", session, map[string]any{"toolName": "bash", "toolCallId": "call_b", "hasUI": ui, "input": map[string]string{"command": "git reset --hard && nix build --no-link"}})
	}
	result := func(session string) []byte {
		return piInput("tool_result", session, map[string]any{"toolName": "bash", "toolCallId": "call_b"})
	}
	var out struct {
		Block   string
		Confirm *struct{ Message, Declined string }
		Append  string
	}
	got := r.run(bash("noui", false))
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil || !strings.Contains(out.Block, "nobody can answer in this mode.\n\n"+heavyNote(50)) || out.Confirm != nil {
		t.Errorf("without a UI the block carries the note: %+v %v", got, err)
	}
	if got := r.run(result("noui")); got.stdout != piAppend("") {
		t.Errorf("the block told it, so the result stays quiet: %+v", got)
	}
	out.Block = ""
	got = r.run(bash("ui", true))
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil || out.Confirm == nil ||
		!strings.HasSuffix(out.Confirm.Declined, "\n\n"+heavyNote(50)) || strings.Contains(out.Confirm.Message, "Power") ||
		!strings.Contains(out.Append, heavyNote(50)) {
		t.Errorf("a dialog carries the note whether approved or declined: %+v %v", got, err)
	}
	if got := r.run(result("ui")); got.stdout != piAppend("") {
		t.Errorf("then quiet: %+v", got)
	}
	got = r.run(piInput("tool_call", "deny", map[string]any{"toolName": "bash", "toolCallId": "call_d", "hasUI": false, "input": map[string]string{"command": "pkill -f x; nix build --no-link"}}))
	if strings.Contains(got.stdout, "Power") {
		t.Errorf("a check's deny drops the note: %+v", got)
	}
	if got := r.run(result("deny")); got.stdout != piAppend("Power: on battery at 50%.") {
		t.Errorf("so the session hears it later: %+v", got)
	}
}

func TestPowerQuietWithoutABatteryOrWhenDisabled(t *testing.T) {
	desktop := newRig(t, "claude-code", true)
	desktop.set("desktop", 0)
	off := newRig(t, "claude-code", false)
	off.set("battery", 20)
	for name, r := range map[string]*rig{"desktop": desktop, "disabled": off} {
		for _, in := range [][]byte{ccPost("s"), ccPrompt("s"), ccBash("s", "nix build --no-link")} {
			if got := r.run(in); got.code != 0 || got.stdout != "" || got.stderr != "" || strings.Contains(got.journal, "power") {
				t.Errorf("%s: want silence, got %+v", name, got)
			}
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(r.journal), "power")); !os.IsNotExist(err) {
			t.Errorf("%s: no state is written: %v", name, err)
		}
	}
}

func piInput(event, session string, extra map[string]any) []byte {
	in := map[string]any{"event": event, "sessionId": session, "cwd": "/home/test/repo", "hasUI": true}
	for k, v := range extra {
		in[k] = v
	}
	b, _ := json.Marshal(in)
	return b
}

func piAppend(text string) string {
	if text == "" {
		return "{}\n"
	}
	b, _ := json.Marshal(map[string]string{"append": "agent-guard: " + text})
	return string(b) + "\n"
}

func TestPowerPi(t *testing.T) {
	r := newRig(t, "pi", true)
	result := func(session string) []byte {
		return piInput("tool_result", session, map[string]any{"toolName": "read", "toolCallId": "call_r"})
	}
	bash := func(session, command string) []byte {
		return piInput("tool_call", session, map[string]any{"toolName": "bash", "toolCallId": "call_b", "input": map[string]string{"command": command}})
	}
	r.steps([]step{
		{"first look on AC", "ac", 90, result("p1"), 0, piAppend("")},
		{"unplugged", "battery", 60, result("p1"), 0, piAppend("Power: unplugged, now on battery at 60%.")},
		{"heavy build", "", 0, bash("p1", "nix build --no-link"), 0, piAppend(heavyNote(60))},
		{"back on AC at the prompt", "ac", 61, piInput("prompt", "p1", nil), 0, piAppend("Power: back on AC, battery at 61%.")},
		{"new session on battery", "battery", 59, piInput("prompt", "p2", nil), 0, piAppend("Power: on battery at 59%.")},
		{"then quiet", "", 0, result("p2"), 0, piAppend("")},
	})
	if j := r.run(result("p2")).journal; !strings.Contains(j, `"harness":"pi","session":"p1","tool_use_id":"call_r","hook":"post-tool","tool":"read","level":"advise","checks":[{"check":"power","level":"advise"}]}`) {
		t.Errorf("pi journal: %s", j)
	}
}

func TestPowerCommand(t *testing.T) {
	r := newRig(t, "claude-code", true)
	r.cfg = filepath.Join(t.TempDir(), "missing.json")
	r.set("battery", 40)
	cases := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{[]string{"power", "status"}, 0, "battery 40%, discharging\n", ""},
		{[]string{"power", "wait", "battery"}, 0, "battery 40%, discharging\n", ""},
		{[]string{"power", "wait", "ac", "-timeout", "30ms"}, 1, "", "agent-guard: power wait ac: timed out after 30ms; now battery 40%, discharging\n"},
		{[]string{"power", "wait", "-timeout", "30ms", "ac"}, 1, "", "agent-guard: power wait ac: timed out after 30ms; now battery 40%, discharging\n"},
		{[]string{"power", "wait", "dc"}, 2, "", "agent-guard: power takes status, or wait ac|battery [-timeout D]\n"},
		{[]string{"power", "wait", "ac", "battery"}, 2, "", "agent-guard: power takes status, or wait ac|battery [-timeout D]\n"},
		{[]string{"power"}, 1, "", "agent-guard: power takes status, or wait ac|battery [-timeout D]\n"},
	}
	for _, c := range cases {
		got := r.run(nil, c.args...)
		if got.code != c.code || got.stdout != c.stdout || got.stderr != c.stderr {
			t.Errorf("%v: want %d %q %q, got %d %q %q", c.args, c.code, c.stdout, c.stderr, got.code, got.stdout, got.stderr)
		}
	}
	r.set("ac", 41)
	if got := r.run(nil, "power", "wait", "ac"); got.code != 0 || got.stdout != "AC, battery 41%, charging\n" {
		t.Errorf("wait ac on AC returns at once: %+v", got)
	}
	r.set("desktop", 0)
	if got := r.run(nil, "power", "status"); got.code != 0 || got.stdout != "AC, no battery\n" {
		t.Errorf("desktop status: %+v", got)
	}
	if got := r.run(nil, "power", "wait", "battery", "-timeout", "1h"); got.code != 1 || got.stderr != "agent-guard: power wait battery: this machine has no battery; now AC, no battery\n" {
		t.Errorf("waiting for battery on a desktop fails at once: %+v", got)
	}
}

func TestPowerExplainAndCheck(t *testing.T) {
	r := newRig(t, "claude-code", true)
	got := r.run(nil, "explain", "nix develop -c sh -c 'nix build .#x'")
	if !strings.HasSuffix(got.stdout, "heavy on battery: nix build\n") {
		t.Errorf("explain names the heavy pattern: %q", got.stdout)
	}
	if got := r.run(nil, "explain", "ls"); strings.Contains(got.stdout, "heavy") {
		t.Errorf("ls is light: %q", got.stdout)
	}
	bad := writeConfig(t, "claude-code", map[string]any{"journal": "/j.jsonl", "power": map[string]any{"enable": true, "heavyCommands": []string{"nix [build"}}})
	env, _, stderr := testEnv(nil)
	if code := Run([]string{"--config", bad, "config", "check"}, env); code != 1 || !strings.Contains(stderr.String(), `power: heavy command pattern "nix [build": syntax error in pattern`) {
		t.Errorf("a bad pattern fails the check, exit %d: %s", code, stderr.String())
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"agent-guard/internal/harness"
	"agent-guard/internal/treesitter/treesittertest"
)

var goldenNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

func testEnv(stdin []byte) (Env, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return Env{
		Stdin:  bytes.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(string) string { return "" },
		Now:    func() time.Time { return goldenNow },
		Getwd:  func() (string, error) { return "/tmp", nil },
	}, &stdout, &stderr
}

func writeConfig(t *testing.T, harness string, extra map[string]any) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	grammars, err := os.ReadFile(treesittertest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]string
	if err := json.Unmarshal(grammars, &g); err != nil {
		t.Fatal(err)
	}
	cfg["grammars"] = g
	cfg["harness"] = harness
	if harness == "pi" {
		cfg["readOnlyRoots"] = []map[string]string{{"path": "/home/test/.claude", "reason": "the Claude Code memory belongs to Claude Code; read it, do not write it."}}
	}
	for k, v := range extra {
		cfg[k] = v
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func argsFor(name string) []string {
	if strings.HasPrefix(name, "memory-") {
		return []string{"confine", "log-forensics"}
	}
	return []string{"hook"}
}

type result struct {
	code           int
	stdout, stderr string
	journal        string
}

func runHook(t *testing.T, harness string, extra map[string]any, args []string, stdin []byte) result {
	t.Helper()
	return runHookEnv(t, harness, extra, nil, args, stdin)
}

func runHookEnv(t *testing.T, harness string, extra map[string]any, vars map[string]string, args []string, stdin []byte) result {
	t.Helper()
	journal := filepath.Join(t.TempDir(), "verdicts.jsonl")
	opts := map[string]any{"journal": journal}
	for k, v := range extra {
		opts[k] = v
	}
	cfg := writeConfig(t, harness, opts)
	env, stdout, stderr := testEnv(stdin)
	env.Getenv = func(k string) string { return vars[k] }
	code := Run(append([]string{"--config", cfg}, args...), env)
	j, err := os.ReadFile(journal)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return result{code, stdout.String(), stderr.String(), string(j)}
}

var updateGoldens = os.Getenv("AGENT_GUARD_UPDATE_GOLDENS") != ""

func golden(t *testing.T, path, got string) string {
	t.Helper()
	if updateGoldens {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return read(t, path)
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHookGoldens(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "hooks", "*.json"))
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range fixtures {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		t.Run(name, func(t *testing.T) {
			got := runHookEnv(t, "claude-code", nil, map[string]string{"CLAUDE_PROJECT_DIR": "/x"}, argsFor(name), []byte(read(t, f)))
			base := filepath.Join("testdata", "claude-code", name)
			want := result{
				code:    mustAtoi(t, golden(t, base+".code", strconv.Itoa(got.code)+"\n")),
				stdout:  golden(t, base+".out", got.stdout),
				stderr:  golden(t, base+".err", got.stderr),
				journal: golden(t, base+".journal", got.journal),
			}
			compare(t, strings.Join(argsFor(name), " "), want, got)
		})
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func compare(t *testing.T, via string, want, got result) {
	t.Helper()
	if got.code != want.code {
		t.Errorf("%s: exit code: want %d, got %d", via, want.code, got.code)
	}
	if got.stdout != want.stdout {
		t.Errorf("%s: stdout:\nwant %q\ngot  %q", via, want.stdout, got.stdout)
	}
	if got.stderr != want.stderr {
		t.Errorf("%s: stderr:\nwant %q\ngot  %q", via, want.stderr, got.stderr)
	}
	if got.journal != want.journal {
		t.Errorf("%s: journal:\nwant %q\ngot  %q", via, want.journal, got.journal)
	}
}

func TestExplainGoldens(t *testing.T) {
	commands := strings.Split(strings.TrimRight(read(t, filepath.Join("testdata", "commands.txt")), "\n"), "\n")
	for _, harness := range []string{"claude-code", "pi"} {
		cfg := writeConfig(t, harness, nil)
		var got bytes.Buffer
		for _, c := range commands {
			env, stdout, stderr := testEnv(nil)
			fmt.Fprintf(&got, "=== %s\n", c)
			code := Run([]string{"--config", cfg, "explain", c}, env)
			got.WriteString(stdout.String())
			got.WriteString(stderr.String())
			if code != 0 {
				fmt.Fprintf(&got, "exit=%d\n", code)
			}
		}
		if want := golden(t, filepath.Join("testdata", harness, "explain.txt"), got.String()); got.String() != want {
			t.Errorf("%s explain differs:\nwant:\n%s\ngot:\n%s", harness, want, got.String())
		}
	}
}

func transcriptAt(t *testing.T, at time.Time) string {
	t.Helper()
	line := fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":"go ahead"}}`, at.UTC().Format(time.RFC3339Nano))
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func permissionInput(transcriptPath, command string) []byte {
	in, _ := json.Marshal(map[string]any{
		"session_id":      "abcd1234-0000-4000-8000-000000000000",
		"transcript_path": transcriptPath,
		"cwd":             "/home/test/repo",
		"hook_event_name": "PermissionRequest",
		"tool_name":       "Bash",
		"tool_input":      map[string]string{"command": command},
		"tool_use_id":     "toolu_permission",
	})
	return in
}

func TestPermissionRequestAlwaysReachesTheUser(t *testing.T) {
	for _, c := range []struct{ name, transcript string }{
		{"recent message", transcriptAt(t, goldenNow.Add(-2*time.Minute))},
		{"message an hour old", transcriptAt(t, goldenNow.Add(-time.Hour))},
		{"no transcript", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := runHook(t, "claude-code", nil, []string{"hook"}, permissionInput(c.transcript, "git reset --hard"))
			if got.code != 0 || got.stdout != "" || got.stderr != "" {
				t.Errorf("want the dialog untouched, got %+v", got)
			}
			if !strings.Contains(got.journal, `"hook":"permission-request","tool":"Bash","kind":"shell","level":"dialog"`) {
				t.Errorf("the dialog is journaled, got %s", got.journal)
			}
		})
	}
}

func TestRetiredIdleOptionsAreRejected(t *testing.T) {
	for _, field := range []string{"askIdleMinutes", "askAwayGraceMinutes"} {
		cfg := writeConfig(t, "claude-code", map[string]any{field: 15})
		env, _, stderr := testEnv(nil)
		if code := Run([]string{"--config", cfg, "config", "check"}, env); code != 1 || !strings.Contains(stderr.String(), field) {
			t.Errorf("%s: want a config error naming it, exit %d: %s", field, code, stderr.String())
		}
	}
}

func TestDiffNeedsNoConfig(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.tsv")
	if err := os.WriteFile(a, []byte("background\t2\tx &\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env, _, stderr := testEnv(nil)
	if code := Run([]string{"--config", filepath.Join(dir, "missing.json"), "diff", a, a}, env); code != 0 {
		t.Errorf("diff must not load the config, exit %d: %s", code, stderr.String())
	}
}

func TestUnknownHarnessIsAConfigError(t *testing.T) {
	cfg := writeConfig(t, "pi-coding-agent", nil)
	env, _, stderr := testEnv([]byte(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`))
	if code := Run([]string{"--config", cfg, "hook"}, env); code != 1 {
		t.Errorf("want exit 1 for an unknown harness, got %d", code)
	}
	if !strings.Contains(stderr.String(), `unknown harness "pi-coding-agent"`) {
		t.Errorf("want the harness named in the error, got %q", stderr.String())
	}
}

func TestExplainShowsTheRewrite(t *testing.T) {
	cfg := writeConfig(t, "claude-code", nil)
	cases := []struct {
		command, rewrite string
	}{
		{"rg -rn foo .", "rewrite: rg -n foo .\n"},
		{"rg -rn foo . && rg -rl bar .", ""},
	}
	for _, c := range cases {
		env, stdout, _ := testEnv(nil)
		if code := Run([]string{"--config", cfg, "explain", c.command}, env); code != 0 {
			t.Fatalf("%q: exit %d", c.command, code)
		}
		out := stdout.String()
		if !strings.Contains(out, "deny: rg-replace") {
			t.Errorf("%q: the deny is still shown:\n%s", c.command, out)
		}
		got := ""
		if i := strings.Index(out, "rewrite: "); i >= 0 {
			got = out[i:]
		}
		if got != c.rewrite {
			t.Errorf("%q: want %q got %q", c.command, c.rewrite, got)
		}
	}
}

func bashInput(command string) []byte {
	in, _ := json.Marshal(map[string]any{
		"session_id":      "abcd1234-0000-4000-8000-000000000000",
		"cwd":             "/home/test/repo",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]string{"command": command},
		"tool_use_id":     "toolu_x",
	})
	return in
}

func TestRewriteRecordsTheFixedCommandsAdvice(t *testing.T) {
	got := runHook(t, "claude-code", nil, []string{"hook"}, bashInput("rg -rn foo . > /home/test/out.txt"))
	if got.code != 0 || !strings.Contains(got.stdout, `"updatedInput"`) || !strings.Contains(got.stdout, "New files outside the agreed scratch area: /home/test/out.txt") {
		t.Fatalf("want a rewrite carrying the location advice, got %+v", got)
	}
	if !strings.Contains(got.journal, `"level":"rewrite","checks":[{"check":"rg-replace","level":"deny","fixed":true},{"check":"location","level":"advise"}]`) {
		t.Errorf("the journal must carry the real verdicts, got %s", got.journal)
	}
}

func TestHookFailures(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		stdin string
		code  int
	}{
		{"unsupported event", []string{"hook"}, `{"hook_event_name":"Stop","tool_name":"Bash","tool_input":{"command":"ls"}}`, 1},
		{"tool_input not an object", []string{"hook"}, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":"ls"}`, 1},
		{"confine without an agent", []string{"confine"}, `{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"/x/.claude/agent-memory/a/n.md"}}`, 2},
		{"confine with bad input", []string{"confine", "log-forensics"}, `not json`, 2},
		{"confine without a path", []string{"confine", "log-forensics"}, `{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{}}`, 2},
	}
	for _, c := range cases {
		got := runHook(t, "claude-code", nil, c.args, []byte(c.stdin))
		if got.code != c.code || got.stderr == "" {
			t.Errorf("%s: want exit %d with a reason on stderr, got %+v", c.name, c.code, got)
		}
	}
}

func TestPiGoldens(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "pi-hooks", "*.json"))
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range fixtures {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		t.Run(name, func(t *testing.T) {
			got := runHook(t, "pi", nil, []string{"hook"}, []byte(read(t, f)))
			base := filepath.Join("testdata", "pi-native", name)
			want := result{
				code:    mustAtoi(t, golden(t, base+".code", strconv.Itoa(got.code)+"\n")),
				stdout:  golden(t, base+".out", got.stdout),
				stderr:  golden(t, base+".err", got.stderr),
				journal: golden(t, base+".journal", got.journal),
			}
			compare(t, "hook", want, got)
		})
	}
}

func TestEveryHarnessHasACodec(t *testing.T) {
	for _, spec := range harness.All() {
		if c, ok := codecs[spec.ID]; !ok || c.hook == nil || c.source == nil {
			t.Errorf("harness %s has no codec", spec.ID)
		}
	}
}

func TestReadOnlyRootsArePerHarness(t *testing.T) {
	got := runHook(t, "claude-code", nil, []string{"hook"}, bashInput("echo x > /home/test/.claude/x"))
	if got.code != 0 || got.stdout != "" {
		t.Errorf("the Claude Code config has no read-only roots, got %+v", got)
	}
}

func TestTranscriptsDirIsRequired(t *testing.T) {
	cfg := writeConfig(t, "claude-code", nil)
	env, _, stderr := testEnv(nil)
	if code := Run([]string{"--config", cfg, "corpus"}, env); code != 1 || !strings.Contains(stderr.String(), "corpus needs a transcripts dir") {
		t.Errorf("want exit 1 naming the missing dir, got %d %q", code, stderr.String())
	}
}

func TestJournalImport(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "verdicts.jsonl")
	if err := os.WriteFile(legacy, []byte(`{"time":"2026-09-28T12:00:00Z","hook":"pre-bash","level":"deny","checks":["pkill"],"input":"pkill -f x"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "pi.jsonl")
	cfg := writeConfig(t, "pi", map[string]any{"journal": target})
	env, stdout, stderr := testEnv(nil)
	if code := Run([]string{"--config", cfg, "journal", "import", legacy}, env); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "read 1, appended 1, duplicates 0, undecodable 0") {
		t.Errorf("counts: %q", stdout.String())
	}
	if got := read(t, target); !strings.HasPrefix(got, `{"harness":"pi","time":`) {
		t.Errorf("the configured harness is stamped: %q", got)
	}
}

func TestConfine(t *testing.T) {
	cfg := writeConfig(t, "claude-code", nil)
	cases := []struct {
		project, cwd, path string
		code               int
	}{
		{"/x", "/x", "/x/.claude/agent-memory-local/log-forensics/n.md", 0},
		{"/x", "/x", "/x/.claude/agent-memory/log-forensics/n.md", 0},
		{"/x", "/elsewhere", "/home/test/.claude/agent-memory/log-forensics/n.md", 0},
		{"/x", "/x", "/x/.claude/agent-memory-local/../../src/main.go", 2},
		{"/x", "/x", "/x/.claude/agent-memory-local/other/n.md", 2},
		{"/x", "/x", "/x/.claude/agent-memory-local/log-forensics", 2},
		{"/x", "/x", "/tmp/.claude/agent-memory/x", 2},
		{"/x", "/x", ".claude/agent-memory-local/log-forensics/n.md", 2},
		{"", "/r", "/r/.claude/agent-memory-local/log-forensics/n.md", 0},
	}
	for _, c := range cases {
		in, _ := json.Marshal(map[string]any{
			"hook_event_name": "PreToolUse", "cwd": c.cwd, "tool_name": "Write",
			"tool_input": map[string]string{"file_path": c.path, "content": "x"},
		})
		env, _, stderr := testEnv(in)
		env.Getenv = func(k string) string {
			if k == "CLAUDE_PROJECT_DIR" {
				return c.project
			}
			return ""
		}
		code := Run([]string{"--config", cfg, "confine", "log-forensics"}, env)
		if code != c.code {
			t.Errorf("%s (project %q, cwd %s): want exit %d got %d: %s", c.path, c.project, c.cwd, c.code, code, stderr.String())
		}
		if code == 2 && !strings.Contains(stderr.String(), "Blocked: this agent writes only under /home/test/.claude/agent-memory/log-forensics, ") {
			t.Errorf("%s: the deny names the roots: %s", c.path, stderr.String())
		}
	}
}

func TestConfigCheck(t *testing.T) {
	cfg := writeConfig(t, "pi", map[string]any{"journal": "/j.jsonl"})
	env, _, stderr := testEnv(nil)
	if code := Run([]string{"--config", cfg, "config", "check"}, env); code != 0 {
		t.Errorf("the test config is valid, exit %d: %s", code, stderr.String())
	}
	bad := writeConfig(t, "pi", map[string]any{"journal": "j.jsonl"})
	env, _, stderr = testEnv(nil)
	if code := Run([]string{"--config", bad, "config", "check"}, env); code != 1 || !strings.Contains(stderr.String(), `journal "j.jsonl" is not an absolute path`) {
		t.Errorf("a relative journal fails the check, exit %d: %s", code, stderr.String())
	}
}

func TestLegacySubcommandsAreGone(t *testing.T) {
	cfg := writeConfig(t, "claude-code", nil)
	for _, sub := range []string{"pre-bash", "pre-edit", "permission-request", "agent-memory-write"} {
		env, _, _ := testEnv([]byte(`{}`))
		if code := Run([]string{"--config", cfg, sub}, env); code != 1 {
			t.Errorf("%s: want the usage and exit 1, got %d", sub, code)
		}
	}
}

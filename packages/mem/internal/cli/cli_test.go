package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/store"
)

type sandbox struct {
	dir  string
	vars map[string]string
	now  time.Time
}

func newSandbox(t *testing.T) *sandbox {
	dir := t.TempDir()
	return &sandbox{
		dir: dir,
		vars: map[string]string{
			"HOME":           dir,
			"XDG_DATA_HOME":  filepath.Join(dir, "data"),
			"XDG_STATE_HOME": filepath.Join(dir, "state"),
			"MEM_CORE":       filepath.Join(dir, "core.md"),
		},
		now: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC),
	}
}

func (sb *sandbox) run(stdin string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	e := env.Env{
		Stdin:    strings.NewReader(stdin),
		Stdout:   &out,
		Stderr:   &errb,
		Getenv:   func(k string) string { return sb.vars[k] },
		Now:      func() time.Time { return sb.now },
		Getwd:    func() (string, error) { return sb.dir, nil },
		Hostname: func() (string, error) { return "SMOK", nil },
	}
	code := Run(args, e)
	return code, out.String(), errb.String()
}

func TestCLI(t *testing.T) {
	sb := newSandbox(t)
	pushBody := "Do not git push by default. After a commit, wait for explicit authorization; every push is its own action."
	dupBody := "Never push to the remote by default: after committing, wait until the user explicitly authorizes the push."
	steps := []struct {
		name   string
		stdin  string
		args   []string
		code   int
		stdout []string
		stderr []string
	}{
		{name: "get before any write", args: []string{"get", "x"}, code: 1, stderr: []string{"no memory database"}},
		{name: "put new", stdin: pushBody, args: []string{"put", "--key", "no-push-default", "--kind", "rule", "--scope", "global",
			"--title", "No git push by default", "--core", "--trigger", `Bash:\bgit\s+push\b`},
			stdout: []string{"no-push-default\tcreated\tversion 1"}},
		{name: "existing key", stdin: pushBody, args: []string{"put", "--key", "no-push-default", "--kind", "rule", "--scope", "global"},
			code: 3, stderr: []string{"already exists", "--update"}},
		{name: "near duplicate", stdin: dupBody, args: []string{"put", "--key", "never-push-unasked", "--kind", "rule", "--scope", "project:nixos-config",
			"--title", "Never push without asking"},
			code: 3, stderr: []string{"duplicate of no-push-default", "no-push-default  similarity", "--update", "--supersedes no-push-default",
				"--distinct no-push-default --why", "nothing was written"}},
		{name: "distinct needs why", stdin: dupBody, args: []string{"put", "--key", "never-push-unasked", "--kind", "rule", "--scope", "global",
			"--distinct", "no-push-default"}, code: 2, stderr: []string{"--why"}},
		{name: "supersede", stdin: dupBody, args: []string{"put", "--key", "never-push-unasked", "--kind", "rule", "--scope", "global",
			"--title", "Never push without asking", "--core", "--supersedes", "no-push-default", "--why", "reworded"},
			stdout: []string{"never-push-unasked\tcreated\tversion 1", "superseded: no-push-default"}},
		{name: "update superseded", stdin: "x", args: []string{"put", "--key", "no-push-default", "--update"},
			code: 3, stderr: []string{"superseded by never-push-unasked"}},
		{name: "update flags only", args: []string{"put", "--key", "never-push-unasked", "--update", "--core=false"},
			stdout: []string{"never-push-unasked\tupdated\tversion 2"}},
		{name: "related printed", stdin: "Pushing to the FreeToken fork is allowed without asking.",
			args:   []string{"put", "--key", "freetoken-fork-push", "--kind", "rule", "--scope", "project:freetoken", "--title", "FreeToken fork push allowed", "--rel", "0.1"},
			stdout: []string{"freetoken-fork-push\tcreated", "check for conflict:", "never-push-unasked"}},
		{name: "state with ttl", stdin: "agent-guard refactor: 7 commits up to c00c712d are not pushed.",
			args: []string{"put", "--key", "agent-guard-push-state", "--kind", "state", "--scope", "project:nixos-config", "--ttl", "3",
				"--verify", "git -C ~/Sync/PC/share/nixos-config status -sb"},
			stdout: []string{"agent-guard-push-state\tcreated"}},
		{name: "ttl on rule", stdin: "x y z", args: []string{"put", "--key", "rule-ttl", "--kind", "rule", "--scope", "global", "--ttl", "3"},
			code: 2, stderr: []string{"ttl applies to state"}},
		{name: "bad kind", stdin: "x", args: []string{"put", "--key", "k", "--kind", "note", "--scope", "global"}, code: 2, stderr: []string{"kind"}},
		{name: "missing key", stdin: "x", args: []string{"put", "--kind", "rule"}, code: 2, stderr: []string{"missing KEY"}},
		{name: "positional conflicts with key", stdin: "x", args: []string{"put", "--key", "k", "--kind", "rule", "body"}, code: 2, stderr: []string{"key given twice"}},
		{name: "get", args: []string{"get", "never-push-unasked"},
			stdout: []string{"key: never-push-unasked", "status: current", "version: 2", "link: never-push-unasked supersedes no-push-default", "---\n" + dupBody}},
		{name: "get flag after key", args: []string{"get", "no-push-default", "--history"},
			stdout: []string{"status: superseded", "superseded_by: never-push-unasked", "=== version 1", "superseded by never-push-unasked: reworded", "trigger: Bash:"}},
		{name: "history alias", args: []string{"history", "never-push-unasked"}, stdout: []string{"=== version 1", "update"}},
		{name: "search", args: []string{"search", "can", "I", "push", "to", "the", "remote", "after", "committing?"}, stdout: []string{"1. never-push-unasked [rule, current, as of 2026-11-07]"}},
		{name: "search all status", args: []string{"search", "--all-status", "git push authorization"}, stdout: []string{"no-push-default [rule, superseded"}},
		{name: "search kind", args: []string{"search", "pushed", "--kind", "state"}, stdout: []string{"1. agent-guard-push-state"}},
		{name: "search mode", args: []string{"search", "--mode", "nope", "x"}, code: 2},
		{name: "search nothing", args: []string{"search", "quantum chromodynamics"}, stdout: []string{"no matches"}},
		{name: "list", args: []string{"list"}, stdout: []string{"agent-guard-push-state\tstate\tcurrent\tproject:nixos-config\t-\t2026-11-07\t", "never-push-unasked\trule\tcurrent\tglobal\t-"}},
		{name: "list status", args: []string{"list", "--status", "superseded"}, stdout: []string{"no-push-default\trule\tsuperseded"}},
		{name: "retire needs why", args: []string{"retire", "freetoken-fork-push"}, code: 2},
		{name: "retire", args: []string{"retire", "freetoken-fork-push", "--why", "fork archived"}, stdout: []string{"freetoken-fork-push\tretired\tversion 2"}},
		{name: "retire again", args: []string{"retire", "freetoken-fork-push", "--why", "x"}, code: 3},
		{name: "unchanged update points to verify", stdin: "agent-guard refactor: 7 commits up to c00c712d are not pushed.",
			args: []string{"put", "--key", "agent-guard-push-state", "--update"}, stdout: []string{"unchanged", "`mem verify agent-guard-push-state`"}},
		{name: "verify", args: []string{"verify", "agent-guard-push-state", "--why", "git status -sb"},
			stdout: []string{"agent-guard-push-state\tverified 2026-11-07", "version 1"}},
		{name: "verify needs a key", args: []string{"verify"}, code: 2},
		{name: "verify superseded", args: []string{"verify", "no-push-default"}, code: 3, stderr: []string{"superseded by never-push-unasked"}},
		{name: "search all chunks flag", args: []string{"search", "--all-chunks", "pushed", "--kind", "state"}, stdout: []string{"1. agent-guard-push-state"}},
		{name: "core stdout", args: []string{"core", "--out", "-"}, stdout: []string{"# Agent memory: core records"}},
		{name: "import dispatch", args: []string{"import-legacy", "--dir", "x"}, code: 2, stderr: []string{"--dir and --project are required"}},
		{name: "eval dispatch", args: []string{"eval"}, code: 2, stderr: []string{"--legacy-dir and --out are required"}},
		{name: "unknown", args: []string{"frobnicate"}, code: 2, stderr: []string{"unknown command"}},
		{name: "no command", args: nil, code: 2, stderr: []string{"usage: mem"}},
		{name: "help", args: []string{"help"}, stdout: []string{"usage: mem"}},
	}
	for _, st := range steps {
		code, out, errs := sb.run(st.stdin, st.args...)
		if code != st.code {
			t.Errorf("%s: exit %d, want %d\nstdout: %s\nstderr: %s", st.name, code, st.code, out, errs)
			continue
		}
		for _, w := range st.stdout {
			if !strings.Contains(out, w) {
				t.Errorf("%s: stdout lacks %q:\n%s", st.name, w, out)
			}
		}
		for _, w := range st.stderr {
			if !strings.Contains(errs, w) {
				t.Errorf("%s: stderr lacks %q:\n%s", st.name, w, errs)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(sb.dir, "data", "agent-memory", "mem.db")); err != nil {
		t.Errorf("default db path: %v", err)
	}
}

func TestCLICoreFile(t *testing.T) {
	sb := newSandbox(t)
	core := sb.vars["MEM_CORE"]
	code, _, errs := sb.run("Never send 0xDF to the BS3 Pro cooler.", "put", "--key", "bs3pro-0xdf", "--kind", "fact",
		"--scope", "global", "--core", "--title", "BS3 Pro 0xDF bricks")
	if code != 0 {
		t.Fatalf("put: %d %s", code, errs)
	}
	data, err := os.ReadFile(core)
	if err != nil || !strings.Contains(string(data), "- `bs3pro-0xdf`: BS3 Pro 0xDF bricks. Never send 0xDF") {
		t.Fatalf("core after put: %q %v", data, err)
	}
	code, _, _ = sb.run(strings.Repeat("я", store.CoreBudget), "put", "--key", "huge", "--kind", "fact", "--scope", "global", "--core")
	if code != 1 {
		t.Fatalf("over budget put exit %d", code)
	}
	code, out, _ := sb.run("", "list", "--json")
	var recs []store.Record
	if code != 0 || json.Unmarshal([]byte(out), &recs) != nil || len(recs) != 1 {
		t.Fatalf("list --json after rejected put: %d %s", code, out)
	}
	code, _, _ = sb.run("", "retire", "bs3pro-0xdf", "--why", "cooler sold")
	data, _ = os.ReadFile(core)
	if code != 0 || strings.Contains(string(data), "bs3pro") {
		t.Fatalf("core after retire: %d %q", code, data)
	}
	out2 := filepath.Join(sb.dir, "elsewhere.md")
	code, out, _ = sb.run("", "--db", filepath.Join(sb.dir, "data", "agent-memory", "mem.db"), "core", "--out", out2)
	if code != 0 || !strings.HasPrefix(out, out2+"\t") {
		t.Fatalf("core --out: %d %s", code, out)
	}
}

func TestCLIDBResolution(t *testing.T) {
	sb := newSandbox(t)
	flagDB := filepath.Join(sb.dir, "flag.db")
	envDB := filepath.Join(sb.dir, "env.db")
	sb.vars["MEM_DB"] = envDB
	if code, _, errs := sb.run("body one two", "put", "--db", flagDB, "--key", "a", "--kind", "fact", "--scope", "global"); code != 0 {
		t.Fatalf("put --db: %s", errs)
	}
	if code, _, errs := sb.run("body three four", "--db", flagDB, "put", "--key", "b", "--kind", "fact", "--scope", "global"); code != 0 {
		t.Fatalf("global --db: %s", errs)
	}
	if code, _, errs := sb.run("body five six", "put", "--key", "c", "--kind", "fact", "--scope", "global"); code != 0 {
		t.Fatalf("MEM_DB: %s", errs)
	}
	for path, want := range map[string]string{flagDB: "a\tfact", envDB: "c\tfact"} {
		code, out, _ := sb.run("", "list", "--db", path)
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("list %s = %d %q, want %q", path, code, out, want)
		}
	}
}

func TestCLIJSON(t *testing.T) {
	sb := newSandbox(t)
	code, out, errs := sb.run("Tailscale socketBufferSize is 16 MB.", "put", "--json", "--key", "ts-buffers", "--kind", "fact", "--scope", "global")
	var res store.PutResult
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || !res.Created || res.Key != "ts-buffers" {
		t.Fatalf("put --json: %d %s %s", code, out, errs)
	}
	code, out, _ = sb.run("", "search", "--json", "socketBufferSize")
	var hits []store.Hit
	if code != 0 || json.Unmarshal([]byte(out), &hits) != nil || len(hits) != 1 || hits[0].Record.Key != "ts-buffers" {
		t.Fatalf("search --json: %d %s", code, out)
	}
	code, out, _ = sb.run("", "get", "--json", "ts-buffers")
	var g getOutput
	if code != 0 || json.Unmarshal([]byte(out), &g) != nil || g.Record.Body != "Tailscale socketBufferSize is 16 MB." {
		t.Fatalf("get --json: %d %s", code, out)
	}
	sb.now = sb.now.Add(30 * 24 * time.Hour)
	code, out, _ = sb.run("", "expire", "--json")
	if code != 0 || strings.TrimSpace(out) != `{
  "expired": []
}` {
		t.Fatalf("expire --json: %d %s", code, out)
	}
}

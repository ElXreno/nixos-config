package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/store"
)

var (
	seedTime = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	hookTime = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
)

type sandbox struct {
	root string
	vars map[string]string
	host string
	now  time.Time
}

const fixtureGitConfig = "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = https://git.example/nixos-config.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sb := &sandbox{
		root: root,
		vars: map[string]string{
			"HOME":               home,
			"XDG_DATA_HOME":      filepath.Join(home, ".local", "share"),
			"XDG_STATE_HOME":     filepath.Join(home, ".local", "state"),
			"MEM_CORE":           filepath.Join(home, ".claude", "memory-core.md"),
			"MEM_PROTECTED_DIRS": filepath.Join(home, ".claude", "projects", "*", "memory") + "::relative/dir",
		},
	}
	repo := filepath.Join(root, "repos", "nixos-config")
	wt := filepath.Join(repo, ".claude", "worktrees", "wt")
	memDir := filepath.Join(home, ".claude", "projects", "p", "memory")
	for _, d := range []string{
		filepath.Join(repo, ".git", "worktrees", "wt"),
		filepath.Join(wt, "modules"),
		filepath.Join(root, "work", "plain"),
		memDir,
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "wt")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(fixtureGitConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(memDir, filepath.Join(home, "notes")); err != nil {
		t.Fatal(err)
	}
	links := filepath.Join(home, "w")
	if err := os.MkdirAll(links, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"dangling":  filepath.Join(memDir, "new.md"),
		"rel":       filepath.Join("..", ".claude", "projects", "p", "memory", "newdir"),
		"elsewhere": filepath.Join(root, "work", "plain", "new.md"),
	} {
		if err := os.Symlink(target, filepath.Join(links, name)); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := os.ReadFile(filepath.Join("testdata", "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "transcript.jsonl"), tr, 0o644); err != nil {
		t.Fatal(err)
	}
	return sb
}

func (sb *sandbox) env(stdin string, stdout, stderr *bytes.Buffer) env.Env {
	return env.Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: stdout,
		Stderr: stderr,
		Getenv: func(k string) string { return sb.vars[k] },
		Now: func() time.Time {
			if !sb.now.IsZero() {
				return sb.now
			}
			return hookTime
		},
		Getwd: func() (string, error) { return sb.root, nil },
		Hostname: func() (string, error) {
			if sb.host != "" {
				return sb.host, nil
			}
			return "SMOK.angora-ide.ts.net", nil
		},
	}
}

func (sb *sandbox) dbPath() string {
	return filepath.Join(sb.vars["XDG_DATA_HOME"], "agent-memory", "mem.db")
}

func (sb *sandbox) logPath() string {
	return filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", LogName)
}

func (sb *sandbox) fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "@ROOT@", sb.root)
}

func (sb *sandbox) run(stdin string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(args, sb.env(stdin, &out, &errb))
	return code, out.String(), errb.String()
}

func (sb *sandbox) lastLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(sb.logPath())
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	return lines[len(lines)-1]
}

func trig(tool, re string) []store.Trigger { return []store.Trigger{{Tool: tool, Regex: re}} }

var seedRecords = []store.Record{
	{Key: "no-push-default", Kind: store.Rule, Scopes: []string{"global"}, Core: true, Title: "No git push by default",
		Body:     "Do not git push by default. After a commit, wait for explicit authorization; every push is its own action.",
		Triggers: trig("Bash", `\bgit\b.*\bpush\b`)},
	{Key: "push-rule-old", Kind: store.Rule, Scopes: []string{"global"}, Status: store.Superseded, Title: "Push after each commit",
		Body: "Push the commit to the remote right after each commit, no authorization needed.", Triggers: trig("Bash", `\bpush\b`)},
	{Key: "smok-apex-d2", Kind: store.Rule, Scopes: []string{"host:smok"}, Core: true, Title: "Apex register 0xD2 is never written on SMOK",
		Body: "Writing EC register 0xD2 through the Apex tool on SMOK hangs the embedded controller until a full power drain."},
	{Key: "kurwa-samsung-fw", Kind: store.Rule, Scopes: []string{"host:kurwa"}, Core: true, Title: "Samsung firmware 1B2QNXH7 is never installed",
		Body:     "Samsung 990 Pro NVMe firmware 1B2QNXH7 breaks the drive on KURWA; the drive stays on its current firmware update level.",
		Triggers: trig("Bash", `1B2QNXH7`)},
	{Key: "impermanence-state-dirs", Kind: store.Fact, Scopes: []string{"project:nixos-config"}, Core: true, Title: "Impermanence needs every state dir",
		Body: "Every new service needs its state directories in the impermanence config.\nA missing one fails silently after deploy."},
	{Key: "agent-guard-push-state", Kind: store.State, Scopes: []string{"project:nixos-config"}, TTLDays: 3,
		Body: "agent-guard refactor: 7 commits up to c00c712d are not pushed."},
	{Key: "bs3pro-0xdf", Kind: store.Rule, Scopes: []string{"global"}, Title: "Command 0xDF is never sent to the BS3 Pro",
		Body: "Command 0xDF bricks the Flydigi BS3 Pro cooler; flydigictl does not send it.", Triggers: trig("Bash", `(?i)\b0xdf\b`)},
	{Key: "zfs-arc-ram", Kind: store.Fact, Scopes: []string{"global"}, Title: "ZFS ARC size after boot",
		Body: "The ZFS ARC grows to half of RAM when zfs_arc_max is unset and gives memory back slowly under pressure.\n" +
			"ARC в ZFS после загрузки занимает половину памяти, если zfs_arc_max не задан, и отдаёт её медленно."},
	{Key: "bt-journal#1", Kind: store.Log, Scopes: []string{"global"},
		Body: "2026-04-12 bluetooth headset pairing on the tablet failed again; journal of the pairing attempts last spring."},
	{Key: "settings-json-copy", Kind: store.Fact, Scopes: []string{"global"}, Title: "settings.json is a copied file",
		Body:     "Claude Code settings.json is installed by home-manager activation as a copy, not a store symlink.",
		Triggers: trig("Write|Edit", `\.claude/settings\.json$`)},
	{Key: "collusion-wiki", Kind: store.Rule, Scopes: []string{"global"}, Title: "The collusion wiki is never opened from the home IP",
		Body: "The collusion wiki logs visitors; it is read only through the proxy.", Triggers: trig("WebFetch", `collusion`)},
}

func (sb *sandbox) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Upsert(ctx, seedRecords, nil, "seed"); err != nil {
		t.Fatal(err)
	}
}

type spec struct {
	name     string
	event    string
	fixture  string
	stdin    string
	args     []string
	empty    bool
	decision string
	has      []string
	lacks    []string
	log      []string
}

type envelope struct {
	HookSpecificOutput Output `json:"hookSpecificOutput"`
}

func (sb *sandbox) check(t *testing.T, sp spec) {
	t.Helper()
	stdin := sp.stdin
	if sp.fixture != "" {
		stdin = sb.fixture(t, sp.fixture)
	}
	code, out, errs := sb.run(stdin, append([]string{sp.event}, sp.args...)...)
	if code != 0 {
		t.Errorf("%s: exit %d, want 0 (stderr %q)", sp.name, code, errs)
	}
	if sp.empty {
		if out != "" {
			t.Errorf("%s: want empty stdout, got %s", sp.name, out)
		}
	} else {
		var env envelope
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Errorf("%s: stdout is not hook JSON: %v\n%s", sp.name, err, out)
			return
		}
		o := env.HookSpecificOutput
		if o.HookEventName != eventNames[sp.event] {
			t.Errorf("%s: hookEventName %q, want %q", sp.name, o.HookEventName, eventNames[sp.event])
		}
		if o.PermissionDecision != sp.decision {
			t.Errorf("%s: permissionDecision %q, want %q", sp.name, o.PermissionDecision, sp.decision)
		}
		body := o.AdditionalContext
		if sp.decision == "deny" {
			body = o.PermissionDecisionReason
			if o.AdditionalContext != "" {
				t.Errorf("%s: deny carries context %q", sp.name, o.AdditionalContext)
			}
		}
		for _, w := range sp.has {
			if !strings.Contains(body, w) {
				t.Errorf("%s: output lacks %q:\n%s", sp.name, w, body)
			}
		}
		for _, w := range sp.lacks {
			if strings.Contains(body, w) {
				t.Errorf("%s: output has %q:\n%s", sp.name, w, body)
			}
		}
	}
	if len(sp.log) > 0 {
		line := sb.lastLog(t)
		for _, w := range sp.log {
			if !strings.Contains(line, w) {
				t.Errorf("%s: hook.log line lacks %q: %s", sp.name, w, line)
			}
		}
	}
}

func TestHooks(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	memReason := []string{"mem put", "only the memory store writes"}
	steps := []spec{
		{name: "session start", event: SessionStart, fixture: "session-start-startup",
			has:   []string{"project:nixos-config and host:smok", "`smok-apex-d2` [rule, as of 2026-10-01]", "`impermanence-state-dirs` [fact, as of 2026-10-01]", "\n  A missing one"},
			lacks: []string{"kurwa-samsung-fw", "no-push-default", "agent-guard-push-state"},
			log:   []string{"session-start session=s1 inject", "expired agent-guard-push-state"}},
		{name: "resume keeps the injected set", event: SessionStart, fixture: "session-start-resume", empty: true,
			log: []string{"session=s1 none"}},
		{name: "compact injects again", event: SessionStart, fixture: "session-start-compact", has: []string{"smok-apex-d2", "impermanence-state-dirs"}},
		{name: "worktree maps to the main repo", event: SessionStart, fixture: "session-start-worktree", has: []string{"project:nixos-config", "impermanence-state-dirs"}},
		{name: "no git root", event: SessionStart, fixture: "session-start-nogit", has: []string{"for host:smok (", "smok-apex-d2"},
			lacks: []string{"impermanence-state-dirs", "project:"}},
		{name: "recall", event: UserPromptSubmit, fixture: "prompt-push",
			has:   []string{"records related to this prompt", "`no-push-default` [rule, as of 2026-10-01]: No git push by default. Do not git push"},
			lacks: []string{"push-rule-old", "bt-journal", "agent-guard-push-state"},
			log:   []string{"user-prompt-submit session=s1 inject", "no-push-default="}},
		{name: "recall dedup", event: UserPromptSubmit, fixture: "prompt-push", empty: true, log: []string{"session=s1 none"}},
		{name: "recall russian", event: UserPromptSubmit, fixture: "prompt-push-ru", has: []string{"zfs-arc-ram", "отдаёт её медленно"}},
		{name: "short prompt takes the last answer", event: UserPromptSubmit, fixture: "prompt-short", has: []string{"zfs-arc-ram"},
			lacks: []string{"quantum"}, log: []string{"query +", "chars of the last answer"}},
		{name: "slash command", event: UserPromptSubmit, fixture: "prompt-slash", empty: true, log: []string{"skip-command"}},
		{name: "unrelated prompt", event: UserPromptSubmit, fixture: "prompt-unrelated", empty: true, log: []string{"session=s1 none"}},
		{name: "other host", event: UserPromptSubmit, fixture: "prompt-kurwa", empty: true},
		{name: "log records are not recalled", event: UserPromptSubmit, fixture: "prompt-log", empty: true},
		{name: "deny write into legacy memory", event: PreToolUse, fixture: "ptu-write-memory", decision: "deny",
			has: append([]string{filepath.Join(sb.root, "home/.claude/projects/*/memory")}, memReason...), log: []string{"pre-tool-use session=s1 deny", "tool Write"}},
		{name: "deny edit of the database", event: PreToolUse, fixture: "ptu-edit-db", decision: "deny",
			has: append([]string{filepath.Join(sb.root, "home/.local/share/agent-memory")}, memReason...)},
		{name: "deny multiedit", event: PreToolUse, fixture: "ptu-multiedit-memory", decision: "deny", has: memReason},
		{name: "deny notebook edit", event: PreToolUse, fixture: "ptu-notebook-memory", decision: "deny", has: []string{"n.ipynb"}},
		{name: "deny dot-dot path", event: PreToolUse, fixture: "ptu-write-dotdot", decision: "deny"},
		{name: "deny through a symlink", event: PreToolUse, fixture: "ptu-write-symlink", decision: "deny"},
		{name: "deny relative path", event: PreToolUse, fixture: "ptu-write-relative", decision: "deny"},
		{name: "deny through a dangling symlink", event: PreToolUse, fixture: "ptu-write-dangling", decision: "deny", has: memReason},
		{name: "deny through a dangling relative dir symlink", event: PreToolUse, fixture: "ptu-write-dangling-rel", decision: "deny"},
		{name: "dangling symlink outside the store", event: PreToolUse, fixture: "ptu-write-dangling-ok", empty: true},
		{name: "deny edit of the core file", event: PreToolUse, fixture: "ptu-edit-core", decision: "deny",
			has: []string{"memory-core.md is generated from the memory store by `mem core`", "mem put"}},
		{name: "sibling dir is not protected", event: PreToolUse, fixture: "ptu-write-sibling", empty: true},
		{name: "plain write", event: PreToolUse, fixture: "ptu-write-ok", empty: true},
		{name: "read is not a write", event: PreToolUse, fixture: "ptu-read-memory", empty: true},
		{name: "file tool trigger", event: PreToolUse, fixture: "ptu-write-settings",
			has: []string{"trigger matches this Write call", "settings-json-copy"}, log: []string{"inject", "tool Write keys settings-json-copy"}},
		{name: "trigger deduped against recall", event: PreToolUse, fixture: "ptu-bash-push", empty: true},
		{name: "subagent has its own set", event: PreToolUse, fixture: "ptu-bash-push-agent",
			has: []string{"`no-push-default` [rule"}, lacks: []string{"push-rule-old"}},
		{name: "no trigger", event: PreToolUse, fixture: "ptu-bash-ls", empty: true},
		{name: "bash trigger", event: PreToolUse, fixture: "ptu-bash-0xdf", has: []string{"trigger matches this Bash call", "bs3pro-0xdf"}},
		{name: "bash trigger dedup", event: PreToolUse, fixture: "ptu-bash-0xdf", empty: true},
		{name: "trigger of another host", event: PreToolUse, fixture: "ptu-bash-kurwa", empty: true},
		{name: "url trigger", event: PreToolUse, fixture: "ptu-webfetch", has: []string{"collusion-wiki"}},
		{name: "event mismatch", event: PreToolUse, fixture: "wrong-event", empty: true, log: []string{"error", `hook_event_name "PostToolUse" sent to pre-tool-use`}},
		{name: "bad json", event: UserPromptSubmit, fixture: "bad-json", empty: true, log: []string{"user-prompt-submit session=- error", "hook input: invalid character"}},
		{name: "empty stdin", event: SessionStart, stdin: "", empty: true, log: []string{"session-start session=- error", "hook input: EOF"}},
		{name: "over budget", event: PreToolUse, fixture: "ptu-write-memory", args: []string{"--budget", "1ns"}, empty: true,
			log: []string{"error", "over the 1ns budget"}},
		{name: "unknown event", event: "post-tool-use", stdin: "{}", empty: true, log: []string{"post-tool-use session=- error", "unknown event"}},
		{name: "bad flag", event: PreToolUse, stdin: "{}", args: []string{"--nope"}, empty: true, log: []string{"pre-tool-use session=- error", "bad flags"}},
	}
	for _, sp := range steps {
		sb.check(t, sp)
	}

	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	rec, err := s.Get(ctx, "agent-guard-push-state")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != store.Expired {
		t.Errorf("state record status %s after session start, want expired", rec.Status)
	}
	core, err := os.ReadFile(sb.vars["MEM_CORE"])
	if err != nil {
		t.Fatalf("core file after expire: %v", err)
	}
	if !strings.Contains(string(core), "no-push-default") || strings.Contains(string(core), "smok-apex-d2") {
		t.Errorf("core file holds the wrong records:\n%s", core)
	}
	agent, err := os.ReadDir(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(agent) < 5 {
		t.Errorf("session files %d, want one per session and one for the subagent", len(agent))
	}
}

func TestNoDatabase(t *testing.T) {
	sb := newSandbox(t)
	for _, sp := range []spec{
		{name: "session start", event: SessionStart, fixture: "session-start-startup", empty: true, log: []string{"session=s1 no-db"}},
		{name: "recall", event: UserPromptSubmit, fixture: "prompt-push", empty: true, log: []string{"no-db"}},
		{name: "deny without a database", event: PreToolUse, fixture: "ptu-write-memory", decision: "deny", has: []string{"mem put"}},
		{name: "no triggers", event: PreToolUse, fixture: "ptu-bash-push", empty: true},
	} {
		sb.check(t, sp)
	}
	if _, err := os.Stat(sb.dbPath()); err == nil {
		t.Error("a hook created the database")
	}
}

func TestHelp(t *testing.T) {
	sb := newSandbox(t)
	for _, args := range [][]string{nil, {"--help"}, {"help"}} {
		code, out, errs := sb.run("", args...)
		if code != 0 || out != "" || !strings.Contains(errs, "usage: mem hook <event>") {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, out, errs)
		}
	}
}

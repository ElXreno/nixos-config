package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mem/internal/store"
)

func TestProjectFromRemote(t *testing.T) {
	cases := []struct {
		url  string
		want string
		ok   bool
	}{
		{"https://github.com/Example-Org/infra.git", "example-org/infra", true},
		{"https://github.com/Example-Org/infra", "example-org/infra", true},
		{"https://github.com/ElXreno/nixpkgs.git/", "elxreno/nixpkgs", true},
		{"git@github.com:ElXreno/nixos-config.git", "elxreno/nixos-config", true},
		{"github.com:ElXreno/nixos-config", "elxreno/nixos-config", true},
		{"ssh://git@gitlab.example-org.ru:2222/example-org/team/parser.git", "example-org/team/parser", true},
		{"git@gitlab.example-org.ru:example-org/sub/parser.git", "example-org/sub/parser", true},
		{"https://user:secret@github.com/a/b.git", "a/b", true},
		{"git://git.example/a/b", "a/b", true},
		{"https://git.example/nixos-config.git", "nixos-config", true},
		{"https://git.example/~user/repo.git", "~user/repo", true},
		{"https://git.example/a//b.git", "a/b", true},
		{"file:///srv/git/x.git", "", false},
		{"/srv/git/x.git", "", false},
		{"../x.git", "", false},
		{"C:/repos/x", "", false},
		{"https://github.com/", "", false},
		{"https://github.com", "", false},
		{"https://git.example/a b/c.git", "", false},
		{"https://git.example/a/../b", "", false},
		{"", "", false},
		{"%%", "", false},
	}
	for _, c := range cases {
		got, ok := projectFromRemote(c.url)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %q %v, want %q %v", c.url, got, ok, c.want, c.ok)
		}
	}
}

func gitRepo(t *testing.T, dir, origin string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = https://github.com/NixOS/nixpkgs.git\n"
	if origin != "" {
		cfg += "[remote \"origin\"]\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n\turl = \"" + origin + "\"\n"
	}
	cfg += "[branch \"main\"]\n\tremote = origin\n"
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectAt(sb *sandbox, cwd string) string {
	return project(sb.env("", nil, nil), Input{Cwd: cwd})
}

func TestProjectIdentity(t *testing.T) {
	sb := newSandbox(t)
	home := sb.vars["HOME"]
	withRemote := filepath.Join(home, "projects", "repos", "github.com", "Example-Org", "infra")
	noRemote := filepath.Join(home, "tmp", "Other", "infra")
	outside := filepath.Join(sb.root, "srv", "Infra")
	gitRepo(t, withRemote, "git@github.com:Example-Org/infra.git")
	gitRepo(t, noRemote, "")
	gitRepo(t, outside, "")
	wtGit := filepath.Join(withRemote, ".git", "worktrees", "feat")
	wt := filepath.Join(home, "wt", "infra-feat")
	for _, d := range []string{wtGit, filepath.Join(wt, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		cwd, want string
	}{
		{withRemote, "example-org/infra"},
		{filepath.Join(wt, "sub"), "example-org/infra"},
		{noRemote, "~/tmp/other/infra"},
		{outside, strings.ToLower(strings.TrimPrefix(outside, "/"))},
		{filepath.Join(sb.root, "repos", "nixos-config"), "nixos-config"},
		{filepath.Join(sb.root, "repos", "nixos-config", ".claude", "worktrees", "wt", "modules"), "nixos-config"},
		{filepath.Join(sb.root, "work", "plain"), ""},
	}
	for _, c := range cases {
		got := projectAt(sb, c.cwd)
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.cwd, got, c.want)
		}
		if got != "" {
			if _, err := store.NormalizeScope(store.ProjectScope(got)); err != nil {
				t.Errorf("%s: %q is not a valid scope: %v", c.cwd, got, err)
			}
		}
	}
	if a, b := projectAt(sb, withRemote), projectAt(sb, noRemote); a == b {
		t.Errorf("two repos named infra share %q", a)
	}
	gitRepo(t, filepath.Join(home), "")
	if got := projectAt(sb, home); got != "~" {
		t.Errorf("a repo at $HOME: %q", got)
	}
}

func TestProjectIdentityInTheHooks(t *testing.T) {
	sb := newSandbox(t)
	home := sb.vars["HOME"]
	repo := filepath.Join(home, "projects", "Infra")
	gitRepo(t, repo, "https://github.com/Example-Org/infra.git")
	sb.seedWith(t, []store.Record{
		{Key: "infra-core", Kind: store.Rule, Scopes: []string{"project:example-org/infra"}, Core: true, Title: "Infra deploys go through the pipeline",
			Body: "Infra deploys go through the pipeline, never kubectl apply by hand.", Triggers: trig("Bash", `kubectl\s+apply`)},
		{Key: "other-infra", Kind: store.Rule, Scopes: []string{"project:infra"}, Core: true, Title: "Another infra repo",
			Body: "Another repository also called infra.", Triggers: trig("Bash", `kubectl\s+apply`)},
	})
	start, _ := json.Marshal(map[string]string{"session_id": "pi1", "cwd": repo, "hook_event_name": "SessionStart", "source": "startup"})
	sb.check(t, spec{name: "core of the remote project", event: SessionStart, stdin: string(start),
		has: []string{"project:example-org/infra and host:smok", "infra-core"}, lacks: []string{"other-infra"}})
	ptu, _ := json.Marshal(map[string]any{"session_id": "pi2", "cwd": repo, "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]string{"command": "kubectl apply -f x.yaml"}})
	sb.check(t, spec{name: "trigger of the remote project", event: PreToolUse, stdin: string(ptu), has: []string{"infra-core"}, lacks: []string{"other-infra"}})
}

var kurwaRecords = []store.Record{
	{Key: "kurwa-dgpu-polling", Kind: store.Rule, Scopes: []string{"host:kurwa", "project:nixos-config"}, Core: true,
		Title:    "The KURWA dGPU is never polled often",
		Body:     "Polling the KURWA dGPU every few seconds with nvidia-smi keeps it out of runtime suspend and drains the battery.",
		Triggers: trig("Bash", `nvidia-smi`)},
	{Key: "smok-other-repo", Kind: store.Rule, Scopes: []string{"host:smok", "project:other-repo"}, Core: true,
		Title: "SMOK rule of another repo", Body: "A SMOK rule that belongs to another repository about nvidia-smi dmon sampling.",
		Triggers: trig("Bash", `nvidia-smi`)},
}

func kurwaSandbox(t *testing.T, host string) *sandbox {
	t.Helper()
	sb := newSandbox(t)
	sb.host = host
	sb.seed(t)
	sb.seedWith(t, kurwaRecords)
	return sb
}

func toolInput(t *testing.T, sb *sandbox, session, agent, command string) string {
	t.Helper()
	m := map[string]any{"session_id": session, "cwd": sb.root + "/repos/nixos-config", "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]string{"command": command}}
	if agent != "" {
		m["agent_id"] = agent
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const kurwaPrompt = "How often may I poll the dGPU with nvidia-smi before it stops reaching runtime suspend on the battery?"

func TestStrictHostScope(t *testing.T) {
	sh := []string{"--strict-host"}
	smok := kurwaSandbox(t, "SMOK.angora-ide.ts.net")
	smok.check(t, spec{name: "smok session start", event: SessionStart, stdin: sessionStartInput(smok, "h1"), args: sh, has: []string{"smok-apex-d2", "smok-other-repo"}, lacks: []string{"kurwa-dgpu-polling"}})
	got := smok.context(t, toolInput(t, smok, "h2", "", "nvidia-smi -q"), append([]string{PreToolUse}, sh...)...)
	if strings.Contains(got, "kurwa-dgpu-polling") || !strings.Contains(got, "smok-other-repo") {
		t.Errorf("smok trigger:\n%s", got)
	}
	if got := smok.context(t, promptInput(t, smok, "h3", kurwaPrompt), append([]string{UserPromptSubmit}, sh...)...); strings.Contains(got, "kurwa-dgpu-polling") {
		t.Errorf("smok prompt:\n%s", got)
	}
	strict := smok.context(t, sessionStartInput(smok, "h4"), SessionStart, "--strict-host", "--strict-project")
	if strings.Contains(strict, "smok-other-repo") || !strings.Contains(strict, "smok-apex-d2") {
		t.Errorf("strict project session start:\n%s", strict)
	}
	if got := smok.context(t, toolInput(t, smok, "h5", "", "nvidia-smi -q"), PreToolUse, "--strict-host", "--strict-project"); got != "" {
		t.Errorf("strict project trigger:\n%s", got)
	}

	kurwa := kurwaSandbox(t, "KURWA")
	kurwa.check(t, spec{name: "kurwa session start", event: SessionStart, stdin: sessionStartInput(kurwa, "k1"), args: sh, has: []string{"kurwa-dgpu-polling", "kurwa-samsung-fw"}, lacks: []string{"smok-apex-d2", "smok-other-repo"}})
	if got := kurwa.context(t, toolInput(t, kurwa, "k2", "", "nvidia-smi -q"), append([]string{PreToolUse}, sh...)...); !strings.Contains(got, "kurwa-dgpu-polling") || strings.Contains(got, "smok-other-repo") {
		t.Errorf("kurwa trigger:\n%s", got)
	}
	if got := kurwa.context(t, promptInput(t, kurwa, "k3", kurwaPrompt), append([]string{UserPromptSubmit}, sh...)...); !strings.Contains(got, "kurwa-dgpu-polling") {
		t.Errorf("kurwa prompt:\n%s", got)
	}
}

func TestHostScopeDefaultIsAboutTheHost(t *testing.T) {
	smok := kurwaSandbox(t, "SMOK.angora-ide.ts.net")
	smok.check(t, spec{name: "smok session start", event: SessionStart, stdin: sessionStartInput(smok, "d1"), has: []string{"kurwa-dgpu-polling", "smok-other-repo", "smok-apex-d2"}})
	got := smok.context(t, toolInput(t, smok, "d2", "", "nvidia-smi -q"), PreToolUse)
	if !strings.Contains(got, "kurwa-dgpu-polling") || !strings.Contains(got, "smok-other-repo") {
		t.Errorf("smok trigger:\n%s", got)
	}
	if got := smok.context(t, promptInput(t, smok, "d3", kurwaPrompt), UserPromptSubmit); !strings.Contains(got, "kurwa-dgpu-polling") {
		t.Errorf("smok prompt about KURWA:\n%s", got)
	}
	if got := smok.context(t, sessionStartInput(smok, "d4"), SessionStart, "--strict-project"); !strings.Contains(got, "kurwa-dgpu-polling") || strings.Contains(got, "smok-other-repo") {
		t.Errorf("strict project alone keeps host records of this project:\n%s", got)
	}
}

func TestHostScopeInTheVectorPool(t *testing.T) {
	sb := kurwaSandbox(t, "SMOK")
	emb := pinEmbedder{cos: map[string]float32{"KURWA dGPU every few seconds": 0.9, "another repository about nvidia-smi": 0.85}}
	sb.indexPinned(t, emb)
	useEmbedder(t, emb)
	if got := sb.prompt(t, "v0", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", "0.8"); !strings.Contains(got, "kurwa-dgpu-polling") {
		t.Errorf("default vector pool on smok drops the KURWA record:\n%s", got)
	}
	args := []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", "0.8", "--shadow-vec-pool", "fused-reg", "--strict-host"}
	got := sb.prompt(t, "v1", args...)
	if strings.Contains(got, "kurwa-dgpu-polling") || !strings.Contains(got, "smok-other-repo") {
		t.Errorf("vector pool on smok:\n%s", got)
	}
	for _, l := range sb.shadowLines(t) {
		for _, h := range l.Hits {
			if h.Key == "kurwa-dgpu-polling" {
				t.Errorf("the shadow pool sees a KURWA record on SMOK")
			}
		}
	}
	if got := sb.prompt(t, "v2", append(args, "--strict-project")...); strings.Contains(got, "smok-other-repo") || strings.Contains(got, "kurwa-dgpu-polling") {
		t.Errorf("strict vector pool:\n%s", got)
	}
}

func trigSeen(t *testing.T, sb *sandbox, session string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "sessions", session+".keys"))
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

func TestTriggerOncePerSession(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	const cmd = "flydigictl raw 0xDF 0x01"
	once := []string{"--trigger-once", "session"}
	if got := sb.context(t, toolInput(t, sb, "t1", "", cmd), append([]string{PreToolUse}, once...)...); !strings.Contains(got, "bs3pro-0xdf") {
		t.Fatalf("first trigger:\n%s", got)
	}
	if !slices.Contains(trigSeen(t, sb, "t1"), triggerMark+"bs3pro-0xdf") {
		t.Errorf("session file %v lacks the trigger marker", trigSeen(t, sb, "t1"))
	}
	if got := sb.context(t, toolInput(t, sb, "t1", "", cmd), append([]string{PreToolUse}, once...)...); got != "" {
		t.Errorf("repeat in the main agent:\n%s", got)
	}
	if got := sb.context(t, toolInput(t, sb, "t1", "sub1", cmd), append([]string{PreToolUse}, once...)...); got != "" {
		t.Errorf("a subagent repeats the trigger of its session:\n%s", got)
	}
	if got := sb.context(t, toolInput(t, sb, "t1", "sub2", cmd), PreToolUse); !strings.Contains(got, "bs3pro-0xdf") {
		t.Errorf("the default is once per agent, a fresh subagent gets the rule:\n%s", got)
	}
	if got := sb.context(t, toolInput(t, sb, "t1", "sub2", cmd), PreToolUse); got != "" {
		t.Errorf("the default repeats within one agent:\n%s", got)
	}

	if got := sb.context(t, toolInput(t, sb, "t2", "sub1", cmd), append([]string{PreToolUse}, once...)...); !strings.Contains(got, "bs3pro-0xdf") {
		t.Fatalf("subagent first:\n%s", got)
	}
	if got := sb.context(t, toolInput(t, sb, "t2", "", cmd), append([]string{PreToolUse}, once...)...); got != "" {
		t.Errorf("the main agent repeats a trigger its subagent got:\n%s", got)
	}
	if got := sb.context(t, toolInput(t, sb, "t2", "sub2", cmd), append([]string{PreToolUse}, once...)...); got != "" {
		t.Errorf("a second subagent repeats it:\n%s", got)
	}
	if got := sb.context(t, toolInput(t, sb, "t2", "", cmd), PreToolUse); got != "" {
		t.Errorf("the main agent got the rule already, default mode:\n%s", got)
	}

	for _, source := range []string{"compact", "clear", "resume"} {
		sess := "t3-" + source
		sb.context(t, toolInput(t, sb, sess, "", cmd), append([]string{PreToolUse}, once...)...)
		start, _ := json.Marshal(map[string]string{"session_id": sess, "cwd": sb.root + "/repos/nixos-config", "hook_event_name": "SessionStart", "source": source})
		sb.context(t, string(start), SessionStart)
		got := sb.context(t, toolInput(t, sb, sess, "sub", cmd), append([]string{PreToolUse}, once...)...)
		if source == "resume" && got != "" {
			t.Errorf("resume reset the trigger set:\n%s", got)
		}
		if source != "resume" && !strings.Contains(got, "bs3pro-0xdf") {
			t.Errorf("%s did not reset the trigger set:\n%s", source, got)
		}
	}
}

func TestTriggerMarkersStayOutOfThePromptPath(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.context(t, toolInput(t, sb, "m1", "", "flydigictl raw 0xDF 0x01"), PreToolUse, "--trigger-once", "session")
	sb.check(t, spec{name: "prompt after a trigger", event: UserPromptSubmit, stdin: promptInput(t, sb, "m1", "Can I git push the commit to the remote now, or do I wait for your authorization first?"),
		has: []string{"no-push-default"}, lacks: []string{"~trig"}})
	if line := sb.lastLog(t); strings.Contains(line, "~trig") {
		t.Errorf("log line %q", line)
	}
	if got := sb.context(t, toolInput(t, sb, "m1", "", "git push origin main"), PreToolUse); got != "" {
		t.Errorf("a record the prompt injected fires its trigger:\n%s", got)
	}
}

func TestTriggerFlagIsChecked(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	code, out, _ := sb.run(toolInput(t, sb, "f1", "", "flydigictl raw 0xDF 0x01"), PreToolUse, "--trigger-once", "never")
	if code != 0 || out != "" || !strings.Contains(sb.lastLog(t), "bad flags") {
		t.Errorf("exit %d stdout %q log %q", code, out, sb.lastLog(t))
	}
}

package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"mem/internal/pending"
	"mem/internal/session"
	"mem/internal/store"
)

const (
	launchPush = pushPrompt
	launchARC  = "Why does ZFS ARC eat all the memory after boot and not give it back under pressure?"
)

func (sb *sandbox) cwd() string { return sb.root + "/repos/nixos-config" }

func (sb *sandbox) launch(t *testing.T, session, tool string, input map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"session_id": session, "cwd": sb.cwd(), "hook_event_name": "PreToolUse",
		"tool_name": tool, "tool_input": input, "tool_use_id": "tu"})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (sb *sandbox) started(t *testing.T, session, agentID, agentType string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"session_id": session, "cwd": sb.cwd(), "hook_event_name": "SubagentStart",
		"agent_id": agentID, "agent_type": agentType, "transcript_path": sb.root + "/transcript.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (sb *sandbox) additional(t *testing.T, out string) string {
	t.Helper()
	if out == "" {
		return ""
	}
	var env envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not hook JSON: %v\n%s", err, out)
	}
	if env.HookSpecificOutput.HookEventName != "SubagentStart" {
		t.Errorf("hookEventName %q", env.HookSpecificOutput.HookEventName)
	}
	return env.HookSpecificOutput.AdditionalContext
}

func (sb *sandbox) agentStart(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	code, out, errs := sb.run(stdin, append([]string{SubagentStart}, args...)...)
	if code != 0 {
		t.Fatalf("exit %d (stderr %q)", code, errs)
	}
	return sb.additional(t, out)
}

func (sb *sandbox) agentLaunch(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	code, out, errs := sb.run(stdin, append([]string{PreToolUse}, args...)...)
	if code != 0 {
		t.Fatalf("exit %d (stderr %q)", code, errs)
	}
	return out
}

func (sb *sandbox) seedTopic(t *testing.T) {
	t.Helper()
	sb.seedWith(t, []store.Record{{Key: "agents-howto", Kind: store.Fact, Scopes: []string{"global", "topic:agents"}, Title: "How agents are launched",
		Body: "Subagents get the recall of their launch prompt."}})
}

func (sb *sandbox) queued(t *testing.T, session string) int {
	t.Helper()
	root := filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", pending.SubDir)
	n := 0
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".json") && !strings.HasPrefix(d.Name(), ".tmp-") {
			n++
		}
		return nil
	})
	return n
}

func TestSubagentGetsTheRecallOfItsLaunchPrompt(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedTopic(t)
	if out := sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush, "description": "d"})); out != "" {
		t.Errorf("the launch itself got context: %s", out)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "subagent queued for Explore keys no-push-default") {
		t.Errorf("log %q", line)
	}
	got := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore"))
	for _, w := range []string{"records related to your task", "`no-push-default` [rule", "Memory topics", "agents"} {
		if !strings.Contains(got, w) {
			t.Errorf("context lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "related to this prompt") {
		t.Errorf("prompt header left in:\n%s", got)
	}
	if strings.Contains(got, launchPush) {
		t.Error("the launch prompt is echoed")
	}
	if n := sb.queued(t, "s1"); n != 0 {
		t.Errorf("%d entries left after the start", n)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "subagent-start session=s1 inject") || !strings.Contains(line, "dequeued for Explore keys no-push-default") {
		t.Errorf("log %q", line)
	}
	turns, err := os.ReadFile(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "turns.jsonl"))
	if err != nil || !strings.Contains(string(turns), `"tool":"SubagentStart:Explore"`) || !strings.Contains(string(turns), `"agent":"a1"`) {
		t.Errorf("turn log %q err %v", turns, err)
	}
}

func TestParallelLaunchesOfTheSameTypeKeepTheirOrder(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.now = hookTime
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	sb.now = hookTime.Add(time.Second)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchARC}))
	sb.now = hookTime.Add(2 * time.Second)
	first := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore"))
	second := sb.agentStart(t, sb.started(t, "s1", "a2", "Explore"))
	third := sb.agentStart(t, sb.started(t, "s1", "a3", "Explore"))
	if !strings.Contains(first, "no-push-default") || strings.Contains(first, "zfs-arc-ram") {
		t.Errorf("first agent:\n%s", first)
	}
	if !strings.Contains(second, "zfs-arc-ram") || strings.Contains(second, "no-push-default") {
		t.Errorf("second agent:\n%s", second)
	}
	if third != "" {
		t.Errorf("third agent got context without a launch:\n%s", third)
	}
}

func TestLaunchesOfDifferentTypesAndSessionsDoNotMix(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	sb.agentLaunch(t, sb.launch(t, "s2", "Agent", map[string]any{"subagent_type": "Plan", "prompt": launchARC}))
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "Plan")); got != "" {
		t.Errorf("another type took the entry:\n%s", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s3", "a1", "Explore")); got != "" {
		t.Errorf("another session took the entry:\n%s", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a2", "Explore")); !strings.Contains(got, "no-push-default") {
		t.Errorf("owner lost its entry:\n%s", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s2", "a3", "Plan")); !strings.Contains(got, "zfs-arc-ram") {
		t.Errorf("owner lost its entry:\n%s", got)
	}
}

func TestOmittedSubagentTypeMeansGeneralPurposeAndTaskIsTheOldName(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"prompt": launchPush}))
	sb.agentLaunch(t, sb.launch(t, "s1", "Task", map[string]any{"subagent_type": "general-purpose", "prompt": launchARC}))
	sb.now = hookTime.Add(time.Second)
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "general-purpose")); got == "" {
		t.Error("omitted subagent_type was not queued as general-purpose")
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a2", "general-purpose")); got == "" {
		t.Error("the Task tool was not queued")
	}
}

func TestForkAndUselessLaunchesQueueNothing(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	for name, in := range map[string]string{
		"fork":          sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "fork", "prompt": launchPush}),
		"no prompt":     sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore"}),
		"blank prompt":  sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": "  \n"}),
		"unrelated":     sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": "quantum chromodynamics of lattice gauge fields"}),
		"other tool":    sb.launch(t, "s1", "Read", map[string]any{"subagent_type": "Explore", "prompt": launchPush}),
		"no session id": sb.launch(t, "", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}),
	} {
		if out := sb.agentLaunch(t, in); out != "" {
			t.Errorf("%s: stdout %s", name, out)
		}
		if n := sb.queued(t, "s1"); n != 0 {
			t.Errorf("%s: %d entries queued", name, n)
		}
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "fork")); got != "" {
		t.Errorf("a fork got context:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "skip-fork") {
		t.Errorf("log %q", line)
	}
}

func TestQueuedRecallExpires(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	sb.now = hookTime.Add(2*time.Minute + time.Second)
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); got != "" {
		t.Errorf("an expired entry was handed over:\n%s", got)
	}
	if n := sb.queued(t, "s1"); n != 0 {
		t.Errorf("expired entry stays queued")
	}
	sb.now = hookTime
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	sb.now = hookTime.Add(40 * time.Second)
	if got := sb.agentStart(t, sb.started(t, "s1", "a2", "Explore"), "--pending-ttl", "30s"); got != "" {
		t.Errorf("--pending-ttl 30s ignored:\n%s", got)
	}
	sb.now = hookTime
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	sb.now = hookTime.Add(110 * time.Second)
	if got := sb.agentStart(t, sb.started(t, "s1", "a3", "Explore")); !strings.Contains(got, "no-push-default") {
		t.Errorf("an entry inside the ttl was dropped:\n%s", got)
	}
}

func TestEmptyQueueEmitsOnlyTheTopicMap(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); got != "" {
		t.Errorf("no topics and no queue still produced context:\n%s", got)
	}
	sb.seedTopic(t)
	got := sb.agentStart(t, sb.started(t, "s1", "a2", "Explore"))
	if !strings.HasPrefix(got, "Memory topics") || strings.Contains(got, "records related") {
		t.Errorf("map-only context:\n%s", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a3", "Explore"), "--no-map"); got != "" {
		t.Errorf("--no-map:\n%s", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a4", "Explore"), "--map-chars", "0"); got != "" {
		t.Errorf("--map-chars 0:\n%s", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a5", "Explore"), "--map-chars", "400"); len([]rune(got)) > 400 {
		t.Errorf("map over its budget: %d chars", len([]rune(got)))
	}
}

func TestSubagentRecallIsMarkedSeenForThatAgentOnly(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	sb.agentStart(t, sb.started(t, "s1", "a1", "Explore"))
	bash := func(agent string) string {
		data, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": sb.cwd(), "hook_event_name": "PreToolUse", "agent_id": agent,
			"tool_name": "Bash", "tool_input": map[string]any{"command": "git push origin main"}})
		return string(data)
	}
	if out := sb.agentLaunch(t, bash("a1")); out != "" {
		t.Errorf("the subagent got the same record again through its trigger: %s", out)
	}
	if out := sb.agentLaunch(t, bash("a2")); !strings.Contains(out, "no-push-default") {
		t.Errorf("another agent lost the trigger: %s", out)
	}
	if again := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); again != "" {
		t.Errorf("a resumed agent was served twice:\n%s", again)
	}
}

func TestResumedAgentDoesNotTakeAnotherAgentsEntry(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedTopic(t)
	sb.agentStart(t, sb.started(t, "s1", "a1", "Explore"))
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	if again := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); again != "" {
		t.Errorf("resume got context:\n%s", again)
	}
	if n := sb.queued(t, "s1"); n != 1 {
		t.Errorf("the resume consumed the queue: %d left", n)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a2", "Explore")); !strings.Contains(got, "no-push-default") {
		t.Errorf("new agent lost its entry:\n%s", got)
	}
}

func TestRealParallelProcessesEachGetOneEntry(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := &sandbox{root: sb.root, vars: sb.vars, now: hookTime.Add(time.Duration(i) * time.Millisecond)}
			s.run(s.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}), PreToolUse)
		}()
	}
	wg.Wait()
	if n := sb.queued(t, "s1"); n != 8 {
		t.Fatalf("%d entries queued by 8 parallel launches", n)
	}
	var mu sync.Mutex
	served := 0
	for i := range 11 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := &sandbox{root: sb.root, vars: sb.vars, now: hookTime.Add(time.Second)}
			_, out, _ := s.run(s.started(t, "s1", "a"+string(rune('a'+i)), "Explore"), SubagentStart)
			if strings.Contains(out, "no-push-default") {
				mu.Lock()
				served++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if served != 8 {
		t.Errorf("%d agents got a recall, want 8", served)
	}
	if n := sb.queued(t, "s1"); n != 0 {
		t.Errorf("%d entries left", n)
	}
}

func TestMalformedInputNeverBlocksOrFails(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedTopic(t)
	long := strings.Repeat("x", 100000)
	inputs := map[string]string{
		"empty":             "",
		"not json":          "{nope",
		"array":             "[1,2]",
		"null":              "null",
		"number":            "42",
		"truncated":         `{"session_id":"s1","agent_type":"Expl`,
		"wrong types":       `{"session_id":5,"agent_type":["x"],"agent_id":{}}`,
		"wrong event":       `{"session_id":"s1","hook_event_name":"PreToolUse","agent_type":"Explore"}`,
		"no ids":            `{"hook_event_name":"SubagentStart"}`,
		"no agent type":     `{"session_id":"s1","hook_event_name":"SubagentStart","agent_id":"a1"}`,
		"huge agent type":   `{"session_id":"s1","hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"` + long + `"}`,
		"odd chars":         `{"session_id":"../../x\u0000","hook_event_name":"SubagentStart","agent_id":"../a\n","agent_type":"é\n\t"}`,
		"unicode surrogate": `{"session_id":"s1","agent_type":"\ud800"}`,
	}
	for name, in := range inputs {
		code, out, _ := sb.run(in, SubagentStart)
		if code != 0 {
			t.Errorf("%s: exit %d", name, code)
		}
		if out != "" {
			var env envelope
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Errorf("%s: stdout is not JSON: %q", name, out)
			}
		}
	}
	launches := map[string]string{
		"input is a string":  `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":"just text"}`,
		"input is null":      `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":null}`,
		"input is an array":  `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":[1]}`,
		"input is missing":   `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent"}`,
		"prompt is a number": `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Task","tool_input":{"prompt":7,"subagent_type":false}}`,
		"prompt is huge":     `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{"prompt":"` + long + `"}}`,
		"type is huge":       `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{"prompt":"` + launchPush + `","subagent_type":"` + long + `"}}`,
	}
	for name, in := range launches {
		code, out, _ := sb.run(in, PreToolUse)
		if code != 0 || out != "" {
			t.Errorf("%s: exit %d stdout %q", name, code, out)
		}
	}
	files, _ := os.ReadDir(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", pending.SubDir))
	for _, f := range files {
		entries, _ := os.ReadDir(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", pending.SubDir, f.Name()))
		for _, e := range entries {
			if len(e.Name()) > 200 {
				t.Errorf("queue file name of %d bytes", len(e.Name()))
			}
		}
	}
}

func TestSubagentHookSurvivesBrokenStateAndStore(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	state := filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", pending.SubDir)
	if err := os.Chmod(state, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(state, 0o700) })
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); got != "" {
		t.Errorf("unreadable queue still produced context:\n%s", got)
	}
	if out := sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush})); out != "" {
		t.Errorf("unwritable queue: %s", out)
	}
	if err := os.Chmod(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sb.dbPath(), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := sb.run(sb.started(t, "s1", "a2", "Explore"), SubagentStart)
	if code != 0 || strings.Contains(out, "panic") {
		t.Errorf("corrupt store: exit %d stdout %q", code, out)
	}
	if out := sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush})); out != "" {
		t.Errorf("corrupt store on launch: %s", out)
	}
}

func TestSubagentHookWithoutDatabase(t *testing.T) {
	sb := newSandbox(t)
	if out := sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush})); out != "" {
		t.Errorf("launch: %s", out)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); got != "" {
		t.Errorf("start:\n%s", got)
	}
	if _, err := os.Stat(sb.dbPath()); err == nil {
		t.Error("a hook created the database")
	}
}

func TestBadSubagentFlags(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	for _, args := range [][]string{{"--pending-ttl", "0"}, {"--pending-ttl", "-1s"}, {"--pending-ttl", "11m"}, {"--nope"}, {"--map-chars", "x"}} {
		code, out, _ := sb.run(sb.started(t, "s1", "a1", "Explore"), append([]string{SubagentStart}, args...)...)
		if code != 0 || out != "" {
			t.Errorf("%v: exit %d stdout %q", args, code, out)
		}
		if line := sb.lastLog(t); !strings.Contains(line, "bad flags") {
			t.Errorf("%v: log %q", args, line)
		}
	}
}

func TestSubagentOverBudgetPrintsNothing(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	code, out, _ := sb.run(sb.started(t, "s1", "a1", "Explore"), SubagentStart, "--budget", "1ns")
	if code != 0 || out != "" {
		t.Errorf("exit %d stdout %q", code, out)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "over the 1ns budget") {
		t.Errorf("log %q", line)
	}
}

func TestSubagentVectorRecallAndTriggerSafety(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	emb := &keywordEmbedder{}
	useEmbedder(t, emb)
	paraphrase := "Should I send my commits upstream already?"
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": paraphrase}), vecFlags...)
	if got, _ := emb.last.Load().(string); got != "Query: "+paraphrase {
		t.Errorf("embedded %q", got)
	}
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "Explore")); !strings.Contains(got, "`no-push-default` [rule") {
		t.Errorf("vector pool missed the paraphrase:\n%s", got)
	}
	before := emb.calls.Load()
	sb.agentLaunch(t, sb.launch(t, "s1", "Bash", map[string]any{"command": "ls"}), vecFlags...)
	sb.agentLaunch(t, sb.launch(t, "s1", "Read", map[string]any{"file_path": "/etc/hosts"}), vecFlags...)
	if emb.calls.Load() != before {
		t.Errorf("non-agent tools called the embedder %d times", emb.calls.Load()-before)
	}
	emb.err = os.ErrDeadlineExceeded
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Plan", "prompt": launchPush}), vecFlags...)
	if got := sb.agentStart(t, sb.started(t, "s1", "a2", "Plan")); !strings.Contains(got, "no-push-default") {
		t.Errorf("an embedder failure lost the lexical fallback:\n%s", got)
	}
}

func TestBrokenVectorFlagsKeepTheTriggerHook(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	bash := `{"session_id":"s9","cwd":"` + sb.cwd() + `","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo 0xdf"}}`
	code, out, _ := sb.run(bash, PreToolUse, "--vec-pool", "reg")
	if code != 0 || !strings.Contains(out, "bs3pro-0xdf") {
		t.Errorf("trigger lost to a vector misconfiguration: exit %d stdout %q", code, out)
	}
	data, _ := os.ReadFile(sb.logPath())
	if !strings.Contains(string(data), "vector off") {
		t.Errorf("no vector off line:\n%s", data)
	}
	if code, out, _ := sb.run(bash, UserPromptSubmit, "--vec-pool", "reg"); code != 0 || out != "" {
		t.Errorf("the prompt hook changed: exit %d stdout %q", code, out)
	}
}

func TestSessionStartPrunesOldQueues(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "old", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	root := filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", pending.SubDir)
	dirs, err := os.ReadDir(root)
	if err != nil || len(dirs) != 1 {
		t.Fatalf("queue dirs %v %v", dirs, err)
	}
	past := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, dirs[0].Name()), past, past); err != nil {
		t.Fatal(err)
	}
	sb.run(sb.fixture(t, "session-start-startup"), SessionStart)
	if left, _ := os.ReadDir(root); len(left) != 0 {
		t.Errorf("%d queue dirs after session start", len(left))
	}
}

func TestSlowRecallNeverCostsTheParentItsTriggers(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	sb.seedWith(t, []store.Record{{Key: "agent-launch-rule", Kind: store.Rule, Scopes: []string{"global"}, Title: "Launch rule",
		Body: "Brief a subagent with the full task.", Triggers: trig("Agent", `(?i)zfs`)}})
	useEmbedder(t, &keywordEmbedder{delay: 5 * time.Second})
	flags := append(slices.Clone(vecFlags), "--embed-timeout", "5s", "--budget", "600ms")
	start := time.Now()
	out := sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchARC}), flags...)
	if !strings.Contains(out, "agent-launch-rule") {
		t.Errorf("the trigger of the launch was lost to the slow recall: %q", out)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("hook took %s", took)
	}
	if n := sb.queued(t, "s1"); n != 1 {
		t.Errorf("%d entries queued, want the lexical fallback", n)
	}
	plain := sb.agentLaunch(t, sb.launch(t, "s2", "Agent", map[string]any{"subagent_type": "fork", "prompt": launchARC}), flags...)
	if plain != out {
		t.Errorf("queueing changed the parent output:\n%s\nvs\n%s", out, plain)
	}
}

func TestStartWithoutAgentIDLeavesTheSessionStateAlone(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "Explore", "prompt": launchPush}))
	main, err := session.Open(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory"), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := main.Add(mapMarker); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"session_id": "s1", "cwd": sb.cwd(), "hook_event_name": "SubagentStart", "agent_type": "Explore"})
	if err != nil {
		t.Fatal(err)
	}
	got := sb.agentStart(t, string(data))
	if !strings.Contains(got, "no-push-default") {
		t.Errorf("an id-less start got nothing:\n%s", got)
	}
	again, err := session.Open(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory"), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if again.Has("no-push-default") {
		t.Error("the subagent recall was marked seen for the parent session")
	}
}

func TestVeryLongAgentTypeStillMatches(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	long := strings.Repeat("x", 300)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": long, "prompt": launchPush}))
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", long)); !strings.Contains(got, "no-push-default") {
		t.Errorf("long type lost its entry:\n%s", got)
	}
}

func TestLaunchTypeSpellingStillReachesTheAgent(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.agentLaunch(t, sb.launch(t, "s1", "Agent", map[string]any{"subagent_type": "general_purpose", "prompt": launchPush}))
	if got := sb.agentStart(t, sb.started(t, "s1", "a1", "general-purpose")); !strings.Contains(got, "no-push-default") {
		t.Errorf("spelling variant lost its entry:\n%s", got)
	}
	for _, spelling := range []string{"Fork", " fork ", "FORK", "f-o_rk"} {
		sb.agentLaunch(t, sb.launch(t, "s2", "Agent", map[string]any{"subagent_type": spelling, "prompt": launchPush}))
	}
	if n := sb.queued(t, "s2"); n != 0 {
		t.Errorf("%d entries queued for fork spellings", n)
	}
}

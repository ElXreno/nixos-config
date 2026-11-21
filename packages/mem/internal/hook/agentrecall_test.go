package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mem/internal/session"
)

const (
	relayFrame = "[Workflow harness \u2014 user request] The harness relays, verbatim and indented below, the user request that triggered this workflow run:\n"
	taskFrame  = "[Workflow harness \u2014 computed task] The task text below was computed at runtime by a workflow script. The computed task text follows:\n"
	relayText  = "  Can I git push the commits upstream now?"
	arcTask    = "  Find out why the ZFS ARC takes half of RAM after boot.\n  \n  Report the numbers."
)

func jsonLine(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func userLine(t *testing.T, agent, content string) string {
	t.Helper()
	return jsonLine(t, map[string]any{"type": "user", "isSidechain": true, "agentId": agent, "message": map[string]any{"role": "user", "content": content}})
}

func (sb *sandbox) agentFile(session, agent string, workflow bool) string {
	dir := filepath.Join(sb.root, "projects", "p", session, "subagents")
	if workflow {
		dir = filepath.Join(dir, "workflows", "wf_1")
	}
	return filepath.Join(dir, "agent-"+agent+".jsonl")
}

func (sb *sandbox) writeAgent(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (sb *sandbox) agentCall(t *testing.T, session, agent, tool string, input map[string]any) string {
	t.Helper()
	return jsonLine(t, map[string]any{"session_id": session, "transcript_path": filepath.Join(sb.root, "projects", "p", session+".jsonl"),
		"cwd": sb.root + "/repos/nixos-config", "hook_event_name": "PreToolUse", "tool_name": tool, "tool_input": input,
		"agent_id": agent, "agent_type": "workflow-subagent"})
}

func agentArgs() []string {
	return append([]string{PreToolUse, "--agent-recall"}, vecFlags...)
}

func agentSandbox(t *testing.T) *sandbox {
	t.Helper()
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	return sb
}

var readCall = map[string]any{"file_path": "/etc/hostname"}

func TestWorkflowAgentGetsRecallAtItsFirstToolCall(t *testing.T) {
	sb := agentSandbox(t)
	transcript := userLine(t, "a1", relayFrame+relayText) + userLine(t, "a1", taskFrame+arcTask) + jsonLine(t, map[string]any{"type": "attachment"})
	sb.writeAgent(t, sb.agentFile("s1", "a1", true), transcript)
	call := sb.agentCall(t, "s1", "a1", "Read", readCall)

	if got := sb.context(t, call, append([]string{PreToolUse}, vecFlags...)...); got != "" {
		t.Fatalf("without --agent-recall: %q", got)
	}
	got := sb.context(t, call, agentArgs()...)
	if !strings.HasPrefix(got, subagentHeader) || !strings.Contains(got, "`zfs-arc-ram` [fact") {
		t.Fatalf("first tool call:\n%s", got)
	}
	if strings.Contains(got, "no-push-default") {
		t.Errorf("the relayed user request was used as the task:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "pre-tool-use session=s1 inject") || !strings.Contains(line, "agent recall for workflow-subagent keys zfs-arc-ram") || !strings.Contains(line, "embed=ok") {
		t.Errorf("log line %q", line)
	}
	turns := sb.turns(t)
	last := turns[len(turns)-1]
	if last.Tool != "AgentRecall:workflow-subagent" || last.Agent != "a1" || last.Status != "inject" || len(last.Keys) != 1 || last.Keys[0] != "zfs-arc-ram" || last.Embed != "ok" {
		t.Errorf("turn %+v", last)
	}
	if again := sb.context(t, call, agentArgs()...); again != "" {
		t.Errorf("second tool call of the same agent: %q", again)
	}
	if other := sb.context(t, sb.agentCall(t, "s1", "a1", "Bash", map[string]any{"command": "zpool status"}), agentArgs()...); other != "" {
		t.Errorf("a later call with another tool: %q", other)
	}
}

func TestWorkflowAgentRecallWaitsForTheTranscript(t *testing.T) {
	sb := agentSandbox(t)
	path := sb.agentFile("s2", "a2", true)
	call := sb.agentCall(t, "s2", "agent-a2", "Read", readCall)
	if got := sb.context(t, call, agentArgs()...); got != "" {
		t.Fatalf("no transcript yet: %q", got)
	}
	sb.writeAgent(t, path, userLine(t, "a2", relayFrame+relayText))
	if got := sb.context(t, call, agentArgs()...); got != "" {
		t.Fatalf("only the relayed request is written: %q", got)
	}
	half := userLine(t, "a2", taskFrame+arcTask)
	sb.writeAgent(t, path, userLine(t, "a2", relayFrame+relayText)+half[:len(half)/2])
	if got := sb.context(t, call, agentArgs()...); got != "" {
		t.Fatalf("a half-written task line: %q", got)
	}
	if session.Claimed(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory"), "s2:agent-a2", session.RecallClaim) {
		t.Fatal("the recall was claimed before the task could be read")
	}
	sb.writeAgent(t, path, userLine(t, "a2", relayFrame+relayText)+half)
	if got := sb.context(t, call, agentArgs()...); !strings.Contains(got, "`zfs-arc-ram` [fact") {
		t.Fatalf("after the task line is complete:\n%s", got)
	}
}

func TestAgentToolSubagentIsLeftToTheQueue(t *testing.T) {
	sb := agentSandbox(t)
	sb.writeAgent(t, sb.agentFile("s3", "a3", false), userLine(t, "a3", "Find out why the ZFS ARC takes half of RAM after boot."))
	call := sb.agentCall(t, "s3", "a3", "Read", readCall)
	if got := sb.context(t, call, agentArgs()...); got != "" {
		t.Fatalf("an Agent tool subagent: %q", got)
	}
	sb.writeAgent(t, sb.agentFile("s3", "a3", true), userLine(t, "a3", taskFrame+arcTask))
	if got := sb.context(t, call, agentArgs()...); got != "" {
		t.Errorf("the decision is taken once per agent: %q", got)
	}
	for _, bad := range []string{"../a3", "a/3", ""} {
		if got := sb.context(t, sb.agentCall(t, "s3", bad, "Read", readCall), agentArgs()...); got != "" {
			t.Errorf("agent id %q: %q", bad, got)
		}
	}
}

func TestWorkflowAgentRecallJoinsATriggerInjection(t *testing.T) {
	sb := agentSandbox(t)
	sb.writeAgent(t, sb.agentFile("s4", "a4", true), userLine(t, "a4", "Find out why the ZFS ARC takes half of RAM after boot."))
	before := len(sb.turns(t))
	got := sb.context(t, sb.agentCall(t, "s4", "a4", "Bash", map[string]any{"command": "git push origin main"}), agentArgs()...)
	recallAt, triggerAt := strings.Index(got, subagentHeader), strings.Index(got, "Agent memory: records whose trigger matches this Bash call.")
	if recallAt != 0 || triggerAt < recallAt || !strings.Contains(got, "`zfs-arc-ram` [fact") || !strings.Contains(got[triggerAt:], "`no-push-default` [rule") {
		t.Fatalf("recall and trigger records in one output:\n%s", got)
	}
	turns := sb.turns(t)[before:]
	if len(turns) != 2 || turns[0].Tool != "AgentRecall:workflow-subagent" || turns[1].Tool != "Bash" || len(turns[1].Keys) != 1 || turns[1].Keys[0] != "no-push-default" {
		t.Errorf("turns %+v", turns)
	}
	again := sb.context(t, sb.agentCall(t, "s4", "a4", "Bash", map[string]any{"command": "zpool status; cat /proc/spl/kstat/zfs/arcstats"}), agentArgs()...)
	if strings.Contains(again, "zfs-arc-ram") {
		t.Errorf("the recalled record is repeated:\n%s", again)
	}
	denied := sb.agentCall(t, "s4", "a5", "Write", map[string]any{"file_path": sb.dbPath(), "content": "x"})
	sb.writeAgent(t, sb.agentFile("s4", "a5", true), userLine(t, "a5", "Find out why the ZFS ARC takes half of RAM after boot."))
	code, out, _ := sb.run(denied, agentArgs()...)
	if code != 0 || !strings.Contains(out, `"permissionDecision":"deny"`) || strings.Contains(out, "zfs-arc-ram") {
		t.Errorf("a denied write carries no recall: %d %s", code, out)
	}
}

func TestTaskText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain prompt", "  Check the ARC size.\n", "Check the ARC size."},
		{"computed task", taskFrame + "  Line one.\n    indented more\n  \n  Line two.", "Line one.\n  indented more\n\nLine two."},
		{"relayed request", relayFrame + relayText, ""},
		{"frame without a body", "[Workflow harness \u2014 computed task] nothing follows", ""},
	}
	for _, c := range cases {
		if got := taskText(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWorkflowAgentRecallSkipsAttachmentBeforeTask(t *testing.T) {
	sb := agentSandbox(t)
	attach := jsonLine(t, map[string]any{"type": "attachment", "attachment": map[string]any{"type": "instructions"}})
	transcript := userLine(t, "a9", relayFrame+relayText) + strings.Repeat(attach, taskLines+1) + userLine(t, "a9", taskFrame+arcTask)
	sb.writeAgent(t, sb.agentFile("s9", "a9", true), transcript)
	got := sb.context(t, sb.agentCall(t, "s9", "a9", "Read", readCall), agentArgs()...)
	if !strings.HasPrefix(got, subagentHeader) || !strings.Contains(got, "`zfs-arc-ram` [fact") {
		t.Fatalf("attachments between relay and task: no recall:\n%q", got)
	}
}

func TestWorkflowAgentTaskStopsAtTheFirstAssistantLine(t *testing.T) {
	dir := t.TempDir()
	attach := jsonLine(t, map[string]any{"type": "attachment"})
	assistant := jsonLine(t, map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": "ok"}})
	toolResult := jsonLine(t, map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "content": "x"}}}})
	cases := []struct{ name, transcript, want string }{
		{"assistant before the task", userLine(t, "a", relayFrame+relayText) + attach + assistant + userLine(t, "a", taskFrame+arcTask), ""},
		{"tool result before the task", userLine(t, "a", relayFrame+relayText) + toolResult + userLine(t, "a", taskFrame+arcTask), ""},
		{"task, attachment, later prompt", userLine(t, "a", taskFrame+"  First.") + attach + userLine(t, "a", "Second.") + assistant + userLine(t, "a", "Third."), "First.\nSecond."},
		{"user lines past the line budget", strings.Repeat(userLine(t, "a", "One."), taskLines) + userLine(t, "a", "Five."), strings.TrimSuffix(strings.Repeat("One.\n", taskLines), "\n")},
	}
	for i, c := range cases {
		path := filepath.Join(dir, "agent-"+string(rune('a'+i))+".jsonl")
		if err := os.WriteFile(path, []byte(c.transcript), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := agentTask(path); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWorkflowAgentRecallSkipsRecordsOfTheTriggerBlock(t *testing.T) {
	sb := agentSandbox(t)
	sb.writeAgent(t, sb.agentFile("s9", "a9", true), userLine(t, "a9", taskFrame+"  Can I git push the commits upstream now? Check whether pushing to the remote is allowed."))
	got := sb.context(t, sb.agentCall(t, "s9", "a9", "Bash", map[string]any{"command": "git push origin main"}), agentArgs()...)
	if !strings.Contains(got, "Agent memory: records whose trigger matches this Bash call.") {
		t.Fatalf("no trigger block:\n%s", got)
	}
	if n := strings.Count(got, "`no-push-default` ["); n != 1 {
		t.Errorf("no-push-default injected %d times in one output:\n%s", n, got)
	}
}

func TestAgentRecallSurvivesAFailedHookRun(t *testing.T) {
	cases := []struct {
		name   string
		broken string
		tool   string
		input  map[string]any
		extra  []string
	}{
		{"trigger-once session, session seen file unreadable", "s5", "Bash", map[string]any{"command": "git push origin main"}, []string{"--trigger-once", "session"}},
		{"default trigger-once agent, agent seen file unreadable", "s5:a5", "Bash", map[string]any{"command": "git push origin main"}, nil},
		{"no trigger, agent seen file unreadable", "s5:a5", "Read", readCall, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := agentSandbox(t)
			sb.writeAgent(t, sb.agentFile("s5", "a5", true), userLine(t, "a5", taskFrame+arcTask))
			stateDir := filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory")
			bad := filepath.Join(stateDir, session.SubDir, session.FileName(c.broken))
			if err := os.MkdirAll(bad, 0o700); err != nil {
				t.Fatal(err)
			}
			args := append(agentArgs(), c.extra...)
			before := len(sb.turns(t))
			code, out, _ := sb.run(sb.agentCall(t, "s5", "a5", c.tool, c.input), args...)
			if code != 0 || out != "" {
				t.Fatalf("the broken run delivered: %d %q", code, out)
			}
			for _, tl := range sb.turns(t)[before:] {
				if tl.Status == "inject" {
					t.Errorf("a turn of the broken run says inject: %+v", tl)
				}
			}
			if session.Claimed(stateDir, "s5:a5", session.RecallClaim) {
				t.Errorf("the broken run kept the recall claim")
			}
			if err := os.Remove(bad); err != nil {
				t.Fatal(err)
			}
			got := sb.context(t, sb.agentCall(t, "s5", "a5", "Read", readCall), args...)
			if !strings.Contains(got, "`zfs-arc-ram` [fact") {
				t.Fatalf("recall lost after a failed run (claimed=%v): %q", session.Claimed(stateDir, "s5:a5", session.RecallClaim), got)
			}
		})
	}
}

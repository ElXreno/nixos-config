package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const reaTool = "mcp__plugin_hm_rea__search_procedures"

func reaRecord(id string) string {
	return `{"evidence_id":"` + id + `","subject":{"name":"hello","identity":{"sha256":"3dbe"}},"analysis_profile":{"digest":"8f13"},"operation":"search_procedures","raw_result":[{"value":"main"}],"normalized_result":[{"value":"main"}],"environment":null,"limitations":["Ghidra reports default auto-analysis complete.","Known provider defect ghidra-9996."],"locations":[],"evidence_links":[]}`
}

func ccMCP(session, agent, tool, response string) []byte {
	extra := map[string]any{"tool_name": tool, "tool_input": map[string]string{"pattern": "main"}, "tool_response": response, "tool_use_id": "toolu_mcp"}
	if agent != "" {
		extra["agent_id"] = agent
	}
	return ccInput("PostToolUse", session, extra)
}

func updated(t *testing.T, stdout string) (string, bool) {
	t.Helper()
	if stdout == "" {
		return "", false
	}
	var out struct {
		Spec struct {
			Event   string  `json:"hookEventName"`
			Updated *string `json:"updatedToolOutput"`
			Context string  `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not hook JSON: %v\n%s", err, stdout)
	}
	if out.Spec.Event != "PostToolUse" {
		t.Errorf("hookEventName = %q", out.Spec.Event)
	}
	if out.Spec.Updated == nil {
		return "", false
	}
	return *out.Spec.Updated, true
}

func TestEvidenceClaudeCode(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "state", "claude-code.jsonl")
	r := &rig{t: t, journal: journal, root: filepath.Join(dir, "power_supply"), cfg: writeConfig(t, "claude-code", map[string]any{
		"journal":       journal,
		"power":         map[string]any{"enable": true},
		"evidenceTools": []string{"mcp__plugin_hm_rea__*"},
	})}
	r.set("ac", 80)

	got := r.run(ccMCP("s1", "", reaTool, reaRecord("ev_1")))
	first, ok := updated(t, got.stdout)
	if !ok || !strings.Contains(first, "Known provider defect") || !strings.Contains(first, `"raw_result":"same as normalized_result"`) {
		t.Fatalf("first result keeps limitations and loses the duplicate raw_result: %q %q", got.stdout, got.stderr)
	}
	if !strings.Contains(got.journal, `"hook":"evidence"`) {
		t.Errorf("a rewrite is journaled: %s", got.journal)
	}

	second, ok := updated(t, r.run(ccMCP("s1", "", reaTool, reaRecord("ev_2"))).stdout)
	if !ok || strings.Contains(second, "Known provider defect") || !strings.Contains(second, `"limitations_shown_earlier":2`) || !strings.Contains(second, `"same_as":{"evidence_id":"ev_1","fields":["subject","analysis_profile"]}`) {
		t.Errorf("second result drops what the agent saw: %s", second)
	}

	sub, _ := updated(t, r.run(ccMCP("s1", "agent1", reaTool, reaRecord("ev_3"))).stdout)
	if !strings.Contains(sub, "Known provider defect") {
		t.Errorf("a subagent gets the limitations once itself: %s", sub)
	}

	if got := r.run(ccInput("PreCompact", "s1", map[string]any{"trigger": "auto"})); got.code != 0 || got.stdout != "" {
		t.Errorf("PreCompact is quiet: %+v", got)
	}
	after, _ := updated(t, r.run(ccMCP("s1", "", reaTool, reaRecord("ev_4"))).stdout)
	if !strings.Contains(after, "Known provider defect") {
		t.Errorf("after compaction the limitations come again: %s", after)
	}

	if _, ok := updated(t, r.run(ccMCP("s1", "", "mcp__plugin_hm_ghidra-mcp__decompile", reaRecord("ev_5"))).stdout); ok {
		t.Error("a tool outside evidenceTools is left alone")
	}
	if _, ok := updated(t, r.run(ccMCP("s1", "", reaTool, `{"result":null}`)).stdout); ok {
		t.Error("a result with nothing to drop is not rewritten")
	}

	r.set("battery", 40)
	out := r.run(ccMCP("s1", "", reaTool, reaRecord("ev_6"))).stdout
	text, ok := updated(t, out)
	if !ok || !strings.Contains(out, "Power: unplugged, now on battery at 40%.") || !strings.Contains(text, `"limitations_shown_earlier":2`) {
		t.Errorf("a power note and a rewrite share one answer: %s", out)
	}
}

func TestEvidenceOffWithoutTools(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "state", "claude-code.jsonl")
	r := &rig{t: t, journal: journal, root: filepath.Join(dir, "power_supply"), cfg: writeConfig(t, "claude-code", map[string]any{
		"journal": journal,
		"power":   map[string]any{"enable": true},
	})}
	r.set("ac", 80)
	if got := r.run(ccMCP("s1", "", reaTool, reaRecord("ev_1"))); got.stdout != "" {
		t.Errorf("without evidenceTools nothing is rewritten: %q", got.stdout)
	}
}

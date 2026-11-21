package claudecode

import (
	"encoding/json"
	"testing"

	"agent-guard/internal/guard"
	"agent-guard/internal/verdict"
)

func decode(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var out struct {
		Spec map[string]json.RawMessage `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatalf("bad JSON %q: %v", s, err)
	}
	return out.Spec
}

func str(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("not a string: %s", raw)
	}
	return s
}

func decision(outcome verdict.Outcome, v ...verdict.Verdict) guard.Decision {
	return guard.Decision{Outcome: outcome, Verdicts: v}
}

var (
	deny    = verdict.Verdict{Level: verdict.Deny, Check: "a", Message: "Blocked: no"}
	ask     = verdict.Verdict{Level: verdict.Ask, Check: "b", Message: "sure?"}
	ask2    = verdict.Verdict{Level: verdict.Ask, Check: "c", Message: "really?"}
	advise  = verdict.Verdict{Level: verdict.Advise, Check: "d", Message: "consider"}
	advise2 = verdict.Verdict{Level: verdict.Advise, Check: "e", Message: "also"}
)

func TestEncode(t *testing.T) {
	if out := Encode(decision(verdict.Silent.Outcome()), nil); out != (Output{}) {
		t.Errorf("silent must produce nothing, got %+v", out)
	}

	out := Encode(decision(verdict.Deny.Outcome(), advise, deny, ask), nil)
	if out.Code != 2 || out.Stdout != "" || out.Stderr != "Blocked: no" {
		t.Errorf("deny must be exit 2 with the deny messages on stderr only, got %+v", out)
	}

	out = Encode(decision(verdict.Ask.Outcome(), advise, ask, ask2), nil)
	spec := decode(t, out.Stdout)
	if out.Code != 0 || out.Stderr != "" || str(t, spec["permissionDecision"]) != "ask" || str(t, spec["permissionDecisionReason"]) != "sure?" || str(t, spec["additionalContext"]) != "really?\n\nconsider" {
		t.Errorf("ask must carry the first ask as reason and the rest as context, got %+v", out)
	}
	if spec := decode(t, Encode(decision(verdict.Ask.Outcome(), ask), nil).Stdout); spec["additionalContext"] != nil {
		t.Errorf("a lone ask carries no context, got %s", spec["additionalContext"])
	}

	out = Encode(decision(verdict.Advise.Outcome(), advise, advise, advise2), nil)
	spec = decode(t, out.Stdout)
	if out.Code != 0 || out.Stderr != "" || spec["permissionDecision"] != nil || str(t, spec["additionalContext"]) != "consider\n\nalso" {
		t.Errorf("advice must be exit 0 with deduplicated context, got %+v", out)
	}
}

func TestEncodeRewrite(t *testing.T) {
	rg := verdict.Verdict{Level: verdict.Deny, Check: "rg-replace", Message: "Blocked: rg -r", Fix: "rg -n foo ."}
	d := guard.Decision{Outcome: verdict.Rewrite, Command: "rg -n foo .", Verdicts: verdict.Verdicts{rg, advise}}
	out := Encode(d, json.RawMessage(`{"command":"rg -rn foo .","description":"search","timeout":60000}`))
	if out.Code != 0 || out.Stderr != "" {
		t.Fatalf("rewrite must be exit 0 with no stderr: %+v", out)
	}
	spec := decode(t, out.Stdout)
	if _, has := spec["permissionDecision"]; has {
		t.Error("a rewrite must not decide permission, the normal checks judge the new input")
	}
	var input map[string]any
	if err := json.Unmarshal(spec["updatedInput"], &input); err != nil {
		t.Fatal(err)
	}
	if input["command"] != "rg -n foo ." || input["description"] != "search" || input["timeout"] != float64(60000) {
		t.Errorf("updatedInput must carry every field with only the command changed: %v", input)
	}
	want := "Ran `rg -n foo .` instead: in rg, -r is --replace and recursion is the default, so the r was dropped. consider"
	if got := str(t, spec["additionalContext"]); got != want {
		t.Errorf("context:\nwant %q\ngot  %q", want, got)
	}
	if out := Encode(d, json.RawMessage(`not json`)); out.Code != 2 || out.Stderr != "Blocked: rg -r" {
		t.Errorf("an unreadable input falls back to the deny, got %+v", out)
	}
}

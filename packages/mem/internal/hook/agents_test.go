package hook

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mem/internal/store"
)

func TestHooksKeepAgentsApart(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }, Access: store.Access{Agent: "deepseek"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Upsert(ctx, []store.Record{
		{Key: "zebra-push-window", Kind: store.Rule, Scopes: []string{"global"}, Title: "Zebra push window",
			Body: "Git push of the zebra commits to the remote waits for the authorization of the lab on Fridays."},
		{Key: "zebra-cooler-bench", Kind: store.Fact, Scopes: []string{"global"}, Title: "Zebra cooler bench",
			Body: "The cooler on the zebra bench is a spare unit with its own firmware.", Triggers: trig("Bash", `(?i)\b0xdf\b`)},
		{Key: "zebra-smok-stand", Kind: store.Rule, Scopes: []string{"host:smok"}, Core: true, Title: "The zebra stand stays on SMOK",
			Body: "The zebra stand is wired to SMOK and is never moved to another host."},
	}, nil, "seed")
	_ = s.Close()
	if err != nil {
		t.Fatal(err)
	}

	for _, sp := range []spec{
		{name: "primary session start", event: SessionStart, fixture: "session-start-startup", has: []string{"smok-apex-d2"}, lacks: []string{"zebra"}},
		{name: "primary recall", event: UserPromptSubmit, fixture: "prompt-push", has: []string{"no-push-default"}, lacks: []string{"zebra"}},
		{name: "primary trigger", event: PreToolUse, fixture: "ptu-bash-0xdf", has: []string{"bs3pro-0xdf"}, lacks: []string{"zebra"}},
	} {
		sb.check(t, sp)
	}

	sb.vars["MEM_AGENT"] = "deepseek"
	delete(sb.vars, "MEM_CORE")
	for _, sp := range []spec{
		{name: "agent session start", event: SessionStart, fixture: "session-start-startup", has: []string{"smok-apex-d2", "zebra-smok-stand"}},
		{name: "agent recall", event: UserPromptSubmit, fixture: "prompt-push", has: []string{"no-push-default", "zebra-push-window"}},
		{name: "agent trigger", event: PreToolUse, fixture: "ptu-bash-0xdf", has: []string{"bs3pro-0xdf", "zebra-cooler-bench"}},
	} {
		sb.check(t, sp)
	}
	state := filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "agents", "deepseek", LogName)
	if _, err := os.Stat(state); err != nil {
		t.Errorf("the agent hook log is not under its own state directory: %v", err)
	}
	if data, _ := os.ReadFile(sb.logPath()); len(data) == 0 || containsWord(string(data), "zebra") {
		t.Errorf("the primary hook log names agent records:\n%s", data)
	}
}

func containsWord(s, w string) bool {
	for i := 0; i+len(w) <= len(s); i++ {
		if s[i:i+len(w)] == w {
			return true
		}
	}
	return false
}

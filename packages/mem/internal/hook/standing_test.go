package hook

import (
	"strings"
	"testing"

	"mem/internal/store"
)

func TestSessionStartStandingRules(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.setTopics(t,
		store.TopicAssign{Key: "zfs-arc-ram", Topic: store.StandingTopic, Title: "Standing rules"},
		store.TopicAssign{Key: "kurwa-samsung-fw", Topic: store.StandingTopic, Title: "Standing rules"},
		store.TopicAssign{Key: "no-push-default", Topic: store.StandingTopic, Title: "Standing rules"},
		store.TopicAssign{Key: "smok-apex-d2", Topic: "smok-hw", Title: "SMOK hardware"},
	)
	got := sessionStartContext(t, sb, "session-start-startup")
	start := strings.Index(got, store.StandingHeader)
	end := strings.Index(got, "Memory topics")
	if start < 0 || end < start {
		t.Fatalf("the standing rules must come before the topic map:\n%s", got)
	}
	block := got[start:end]
	if !strings.Contains(block, "- `zfs-arc-ram`: ZFS ARC size after boot\n") {
		t.Errorf("a global standing rule is missing:\n%s", block)
	}
	for _, w := range []string{"kurwa-samsung-fw", "no-push-default", "grows to half"} {
		if strings.Contains(block, w) {
			t.Errorf("the block names %q (another host, a core record, or a body):\n%s", w, block)
		}
	}
	if line := sb.lastLog(t); !strings.Contains(line, "standing 1 rules") {
		t.Errorf("log %q", line)
	}
	if again := sessionStartContext(t, sb, "session-start-resume"); again != "" {
		t.Errorf("resume repeats the standing rules:\n%s", again)
	}
	if again := sessionStartContext(t, sb, "session-start-compact"); !strings.Contains(again, store.StandingHeader) {
		t.Errorf("compact does not bring the standing rules back:\n%s", again)
	}
}

func TestSessionStartWithoutStandingRules(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.setTopics(t, seedTopics...)
	if got := sessionStartContext(t, sb, "session-start-startup"); strings.Contains(got, "standing rules") {
		t.Errorf("an empty standing topic prints a block:\n%s", got)
	}
}

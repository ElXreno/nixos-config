package hook

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

func (sb *sandbox) setTopics(t *testing.T, as ...store.TopicAssign) {
	t.Helper()
	s, err := store.Open(context.Background(), sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.SetTopics(context.Background(), as); err != nil {
		t.Fatal(err)
	}
}

var seedTopics = []store.TopicAssign{
	{Key: "smok-apex-d2", Topic: "smok-hw", Title: "SMOK hardware"},
	{Key: "no-push-default", Topic: "git", Title: "Git habits"},
	{Key: "push-rule-old", Topic: "git", Title: "Git habits"},
	{Key: "bs3pro-0xdf", Topic: "flydigi", Title: "Flydigi cooler"},
	{Key: "kurwa-samsung-fw", Topic: "kurwa-hw", Title: "KURWA hardware"},
	{Key: "zfs-arc-ram", Topic: "zfs", Title: "ZFS tuning"},
	{Key: "bt-journal#1", Topic: "zfs", Title: "ZFS tuning"},
}

func sessionStartContext(t *testing.T, sb *sandbox, fixture string, args ...string) string {
	t.Helper()
	code, out, errs := sb.run(sb.fixture(t, fixture), append([]string{SessionStart}, args...)...)
	if code != 0 || errs != "" {
		t.Fatalf("exit %d, stderr %q", code, errs)
	}
	if out == "" {
		return ""
	}
	var env envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	return env.HookSpecificOutput.AdditionalContext
}

func TestSessionStartTopicMapAndUsage(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.setTopics(t, seedTopics...)
	got := sessionStartContext(t, sb, "session-start-startup")
	if !strings.HasPrefix(got, store.UsageHeader+"\n") {
		t.Errorf("usage header is not first:\n%s", got)
	}
	core := strings.Index(got, "`smok-apex-d2` [rule")
	topics := strings.Index(got, "Memory topics")
	if core < 0 || topics < core {
		t.Errorf("the map must follow the core records:\n%s", got)
	}
	for _, w := range []string{"- Git habits [git] 1: default, push\n", "- SMOK hardware [smok-hw] 1: 0xD2, Apex, register, written\n", "- ZFS tuning [zfs] 1: ARC, boot, size\n",
		"- Flydigi cooler [flydigi] 1: 0xDF, BS3, Command, Pro, sent\n", "- KURWA hardware [kurwa-hw] 1: 1B2QNXH7, firmware, installed, Samsung\n", "`mem search QUERY --scope topic:ID`"} {
		if !strings.Contains(got, w) {
			t.Errorf("map lacks %q:\n%s", w, got)
		}
	}
	for _, w := range []string{"bt-journal", "push-rule-old", "journal", "Push after"} {
		if strings.Contains(got, w) {
			t.Errorf("map names %q:\n%s", w, got)
		}
	}
	if m := got[max(topics, 0):]; strings.Contains(m, "no-push-default") || strings.Contains(m, "smok-apex-d2") || strings.Contains(m, "zfs-arc-ram") {
		t.Errorf("the map lists record keys:\n%s", m)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "inject") || !strings.Contains(line, "map 5 topics") {
		t.Errorf("log %q", line)
	}
	if again := sessionStartContext(t, sb, "session-start-resume"); again != "" {
		t.Errorf("resume repeats the usage header and the map:\n%s", again)
	}
	if again := sessionStartContext(t, sb, "session-start-compact"); !strings.Contains(again, "Memory topics") || !strings.HasPrefix(again, store.UsageHeader) {
		t.Errorf("compact does not bring the map back:\n%s", again)
	}
}

func TestSessionStartMapFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		has      []string
		lacks    []string
		maxChars int
	}{
		{"no usage", []string{"--no-usage"}, []string{"Memory topics", "smok-apex-d2"}, []string{"Memory lives in mem"}, 0},
		{"no map", []string{"--map-chars", "0"}, []string{"Memory lives in mem", "smok-apex-d2"}, []string{"Memory topics"}, 0},
		{"neither", []string{"--no-usage", "--map-chars", "0"}, []string{"smok-apex-d2"}, []string{"Memory lives in mem", "Memory topics"}, 0},
		{"small cap", []string{"--map-chars", "160"}, []string{"Memory topics", "more"}, []string{"no-push-default"}, 160},
		{"one keyword", []string{"--map-keywords", "1"}, []string{"- Flydigi cooler [flydigi] 1: 0xDF\n", "- Git habits [git] 1: default\n"}, []string{"BS3", "push\n"}, 0},
		{"no keywords", []string{"--map-keywords", "0"}, []string{"- Flydigi cooler [flydigi] 1\n", "- Git habits [git] 1\n"}, []string{"0xDF", "default"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.setTopics(t, seedTopics...)
			got := sessionStartContext(t, sb, "session-start-startup", c.args...)
			for _, w := range c.has {
				if !strings.Contains(got, w) {
					t.Errorf("lacks %q:\n%s", w, got)
				}
			}
			for _, w := range c.lacks {
				if strings.Contains(got, w) {
					t.Errorf("has %q:\n%s", w, got)
				}
			}
			if c.maxChars > 0 {
				i := strings.Index(got, "Memory topics")
				if n := len([]rune(got[i:])); n > c.maxChars {
					t.Errorf("map is %d chars, cap %d", n, c.maxChars)
				}
			}
		})
	}
}

func TestSessionStartWithoutTopicsKeepsTheUsageHeader(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	got := sessionStartContext(t, sb, "session-start-startup")
	if !strings.HasPrefix(got, store.UsageHeader) || strings.Contains(got, "Memory topics") || !strings.Contains(got, "smok-apex-d2") {
		t.Errorf("%s", got)
	}
}

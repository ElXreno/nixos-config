package stats

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"mem/internal/statefile"
	"mem/internal/store"
)

var now = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func stamp(ago time.Duration) string {
	return now.Add(-ago).Format(time.RFC3339)
}

func TestPercentile(t *testing.T) {
	cases := []struct {
		name string
		in   []int64
		p    float64
		want int64
	}{
		{name: "empty", p: 0.5, want: 0},
		{name: "one", in: []int64{7}, p: 0.99, want: 7},
		{name: "median of four", in: []int64{1, 2, 3, 4}, p: 0.5, want: 2},
		{name: "p90 of ten", in: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, p: 0.9, want: 9},
		{name: "p99 of ten", in: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, p: 0.99, want: 10},
		{name: "p50 of five", in: []int64{10, 20, 30, 40, 50}, p: 0.5, want: 30},
	}
	for _, c := range cases {
		if got := Percentile(c.in, c.p); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func TestLatencyOfUnsorted(t *testing.T) {
	got := LatencyOf([]int64{50, 10, 40, 20, 30})
	want := Latency{P50: 30, P90: 50, P99: 50, Max: 50}
	if got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
}

func TestPromptsOf(t *testing.T) {
	k := []statefile.Key{{Key: "a", Cos: 0.7}}
	turns := []statefile.Turn{
		{Event: statefile.EventPrompt, Status: "skipped"},
		{Event: statefile.EventPrompt, Status: "inject", Strong: k, Embed: "ok", Rerank: "ok", TotalMs: 100, Chars: 300},
		{Event: statefile.EventPrompt, Status: "inject", Lexical: []string{"b"}, Embed: "down", Notice: "degraded", TotalMs: 20, Chars: 50},
		{Event: statefile.EventPrompt, Status: "inject", Pointers: k, Embed: "ok", Rerank: "ok", TotalMs: 300, Chars: 40},
		{Event: statefile.EventPrompt, Status: "none", Embed: "ok", Rerank: "timeout", TotalMs: 10},
		{Event: statefile.EventPrompt, Status: "error", TotalMs: 2000},
		{Event: statefile.EventTool, Status: "inject", Chars: 999},
	}
	got := PromptsOf(turns)
	want := Prompts{
		Total: 5, Skipped: 1, Injected: 3, None: 1, Errors: 1, WithText: 2, PointersOnly: 1, Notices: 1,
		Embed:   map[string]int{"ok": 3, "down": 1, "-": 1},
		Rerank:  map[string]int{"ok": 2, "timeout": 1, "-": 2},
		TotalMs: Latency{P50: 100, P90: 2000, P99: 2000, Max: 2000},
		Chars:   390,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%+v\nwant %+v", got, want)
	}
}

func TestToolsOf(t *testing.T) {
	turns := []statefile.Turn{
		{Event: statefile.EventTool, Status: "inject", Session: "s1", Tool: "Bash", Keys: []string{"push", "ssh"}, Chars: 100,
			Fired: []statefile.Fired{{Key: "push", Tool: "Bash", Regex: `git\s+push`}, {Key: "ssh", Tool: "Bash", Regex: `\bssh\b`}}},
		{Event: statefile.EventTool, Status: "inject", Session: "s1", Agent: "ag", Tool: "Bash", Keys: []string{"push"}, Chars: 60,
			Fired: []statefile.Fired{{Key: "push", Tool: "Bash", Regex: `push --force`}}},
		{Event: statefile.EventTool, Status: "inject", Session: "s2", Tool: "Edit", Keys: []string{"push"}, Chars: 40,
			Fired: []statefile.Fired{{Key: "push", Tool: "Bash", Regex: `git\s+push`}}},
		{Event: statefile.EventTool, Status: "inject", Session: "s2", Tool: "Read", Keys: []string{"alpha"}, Chars: 10},
		{Event: statefile.EventTool, Status: "inject", Session: "s1", Agent: "x", Tool: "SubagentStart:explore", Keys: []string{"k"}, Chars: 500},
		{Event: statefile.EventTool, Status: "inject", Session: "s1", Agent: "y", Tool: "SubagentStart:general", Chars: 900},
		{Event: statefile.EventPrompt, Status: "inject", Keys: []string{"push"}},
		{Event: statefile.EventTool, Status: "inject", Session: "s3", Agent: "w", Tool: "AgentRecall:workflow-subagent", Keys: []string{"push"}, Chars: 700},
		{Event: statefile.EventTool, Status: "inject", Session: "s3", Tool: "ToolFailure:Bash", Keys: []string{"push", "ssh"}, Chars: 200,
			Pointers: []statefile.Key{{Key: "push"}, {Key: "ssh"}}},
		{Event: statefile.EventTool, Status: "none", Session: "s3", Tool: "ToolFailure:Bash"},
	}
	tr, sa, fl := ToolsOf(turns)
	want := Triggers{Injections: 4, Chars: 210, Main: 3, Subagent: 1, Keys: []TriggerKey{
		{Key: "push", Fires: 3, Sessions: 2, Regex: []string{`git\s+push`, `push --force`}},
		{Key: "alpha", Fires: 1, Sessions: 1, Regex: []string{}},
		{Key: "ssh", Fires: 1, Sessions: 1, Regex: []string{`\bssh\b`}},
	}}
	if !reflect.DeepEqual(tr, want) {
		t.Errorf("triggers %+v\nwant %+v", tr, want)
	}
	if want := (Subagents{Starts: 2, WithRecall: 1, MapOnly: 1, LazyRecall: 1}); sa != want {
		t.Errorf("subagents %+v, want %+v", sa, want)
	}
	if want := (Failures{Injections: 1, Pointers: 2}); fl != want {
		t.Errorf("failures %+v, want %+v", fl, want)
	}
}

func TestLeftOut(t *testing.T) {
	cases := []struct {
		name, line string
		want       []string
	}{
		{name: "none", line: "2026-11-07T23:13:02Z user-prompt-submit session=s inject 3ms hits 2"},
		{name: "pointers in the middle", line: "T user-prompt-submit session=s inject 390ms hits 12; pointers over 2500 chars, left out a,b_c,d.e; keys x=0.6",
			want: []string{"a", "b_c", "d.e"}},
		{name: "trigger at the end", line: "T pre-tool-use session=s inject 520ms over 2500 chars, left out k1,k2", want: []string{"k1", "k2"}},
		{name: "two lists", line: "T user-prompt-submit session=s inject 1ms over 2000 chars, left out a; pointers over 2500 chars, left out b,c",
			want: []string{"a", "b", "c"}},
		{name: "cut key dropped", line: "T pre-tool-use session=s inject 1ms over 2500 chars, left out k1,k2,kurwa_dcde...", want: []string{"k1", "k2"}},
		{name: "cut after comma", line: "T pre-tool-use session=s inject 1ms over 2500 chars, left out k1,...", want: []string{"k1"}},
	}
	for _, c := range cases {
		if got := LeftOut(c.line); !slices.Equal(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDroppedOf(t *testing.T) {
	got := DroppedOf([]string{
		"T e session=s inject 1ms over 2500 chars, left out b,a",
		"T e session=s inject 1ms pointers over 2500 chars, left out a",
		"T e session=s none 1ms hits 0",
	})
	want := Dropped{Events: 2, Keys: []DroppedKey{{Key: "a", Times: 2}, {Key: "b", Times: 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%+v, want %+v", got, want)
	}
}

func TestComputeWindow(t *testing.T) {
	turns := []statefile.Turn{
		{Time: stamp(time.Hour), Event: statefile.EventPrompt, Status: "none"},
		{Time: stamp(6 * day), Event: statefile.EventSession, Status: "inject", Chars: 70},
		{Time: stamp(8 * day), Event: statefile.EventPrompt, Status: "inject"},
		{Time: "garbage", Event: statefile.EventPrompt, Status: "none"},
		{Time: now.Add(time.Hour).Format(time.RFC3339), Event: statefile.EventPrompt, Status: "none"},
	}
	log := []string{
		stamp(time.Hour) + " pre-tool-use session=s inject 1ms over 2500 chars, left out a",
		stamp(10*day) + " pre-tool-use session=s inject 1ms over 2500 chars, left out b",
	}
	cases := []struct {
		name                      string
		days                      int
		prompts, sessions, events int
		since                     string
	}{
		{name: "last week", days: 7, prompts: 1, sessions: 1, events: 1, since: "2026-10-31T12:00:00Z"},
		{name: "last day", days: 1, prompts: 1, events: 1, since: "2026-11-06T12:00:00Z"},
		{name: "everything", days: 0, prompts: 4, sessions: 1, events: 2},
	}
	for _, c := range cases {
		r := Compute(c.days, now, turns, log)
		if r.Prompts.Total != c.prompts || r.Sessions.Starts != c.sessions || r.Dropped.Events != c.events || r.Since != c.since ||
			r.Until != "2026-11-07T12:00:00Z" || r.Days != c.days {
			t.Errorf("%s: prompts %d sessions %d events %d since %q until %q days %d", c.name,
				r.Prompts.Total, r.Sessions.Starts, r.Dropped.Events, r.Since, r.Until, r.Days)
		}
	}
}

func TestStateOf(t *testing.T) {
	at := func(d int) time.Time { return now.Add(time.Duration(d) * day) }
	recs := []store.Record{
		{Key: "soon", Kind: store.State, Status: store.Current, TTLDays: 3, UpdatedAt: at(-1)},
		{Key: "past-ttl", Kind: store.State, Status: store.Current, TTLDays: 1, UpdatedAt: at(-5), VerifiedAt: at(-4)},
		{Key: "mid", Kind: store.State, Status: store.Current, TTLDays: 14, UpdatedAt: at(-4)},
		{Key: "far", Kind: store.State, Status: store.Current, TTLDays: 30, UpdatedAt: at(0), VerifiedAt: at(0)},
		{Key: "default-ttl", Kind: store.State, Status: store.Current, UpdatedAt: at(-1)},
		{Key: "gone", Kind: store.State, Status: store.Expired, TTLDays: 2, UpdatedAt: at(-30)},
		{Key: "old", Kind: store.State, Status: store.Superseded, UpdatedAt: at(-30)},
		{Key: "rule", Kind: store.Rule, Status: store.Current, UpdatedAt: at(-30)},
	}
	got := StateOf(recs, now)
	date := func(d int) string { return at(d).Local().Format(time.DateOnly) }
	want := State{Current: 5, Expired: 1, Expiring7d: 2, Expiring14d: 4, NeverVerified: 3, Soon: []Expiry{
		{Key: "past-ttl", Expires: date(-3)},
		{Key: "soon", Expires: date(2)},
		{Key: "mid", Expires: date(10)},
		{Key: "default-ttl", Expires: date(13)},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%+v\nwant %+v", got, want)
	}
}

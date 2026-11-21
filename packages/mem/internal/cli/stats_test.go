package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/statefile"
	"mem/internal/stats"
)

func (sb *sandbox) stateDir() string {
	return filepath.Join(sb.dir, "state", env.DirName)
}

func (sb *sandbox) appendTurns(t *testing.T, name string, turns ...statefile.Turn) {
	t.Helper()
	for _, tn := range turns {
		if err := statefile.Append(sb.stateDir(), name, statefile.MaxTurnsSize, tn); err != nil {
			t.Fatal(err)
		}
	}
}

func (sb *sandbox) ago(d time.Duration) string {
	return sb.now.Add(-d).UTC().Format(time.RFC3339)
}

func (sb *sandbox) stats(t *testing.T, args ...string) stats.Report {
	t.Helper()
	code, out, errs := sb.run("", append([]string{"stats", "--json"}, args...)...)
	var r stats.Report
	if code != 0 {
		t.Fatalf("stats %v: exit %d: %s", args, code, errs)
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("stats %v: %v\n%s", args, err, out)
	}
	return r
}

func TestStatsCommand(t *testing.T) {
	sb := newSandbox(t)
	for _, put := range []struct {
		body string
		args []string
	}{
		{"BIMBA deploy of the grafana dashboards waits for the cert rotation.", []string{"--key", "deploy-state", "--kind", "state", "--ttl", "3"}},
		{"Postgres 17 migration on GRATE runs in the staging schema first.", []string{"--key", "migration-state", "--kind", "state", "--ttl", "10"}},
		{"The Minecraft modpack lock stays pinned until the next season.", []string{"--key", "long-state", "--kind", "state", "--ttl", "30"}},
		{"Never git push without an explicit request from the owner.", []string{"--key", "push-rule", "--kind", "rule"}},
	} {
		if code, _, errs := sb.run(put.body, append([]string{"put", "--scope", "global"}, put.args...)...); code != 0 {
			t.Fatalf("%v: %s", put.args, errs)
		}
	}
	day := 24 * time.Hour
	strong := []statefile.Key{{Key: "push-rule", Cos: 0.7}}
	sb.appendTurns(t, statefile.TurnsName+statefile.OldSuffix,
		statefile.Turn{Time: sb.ago(3 * day), Event: statefile.EventPrompt, Session: "s0", Status: "inject", Strong: strong, Embed: "ok", Rerank: "ok", TotalMs: 900, Chars: 400},
		statefile.Turn{Time: sb.ago(20 * day), Event: statefile.EventPrompt, Session: "s0", Status: "none", Embed: "ok", TotalMs: 5},
	)
	sb.appendTurns(t, statefile.TurnsName,
		statefile.Turn{Time: sb.ago(time.Hour), Event: statefile.EventSession, Session: "s1", Status: "inject", Chars: 2000},
		statefile.Turn{Time: sb.ago(time.Hour), Event: statefile.EventPrompt, Session: "s1", Status: "skipped"},
		statefile.Turn{Time: sb.ago(time.Hour), Event: statefile.EventPrompt, Session: "s1", Status: "inject", Pointers: strong, Embed: "down", TotalMs: 30, Chars: 80},
		statefile.Turn{Time: sb.ago(time.Hour), Event: statefile.EventPrompt, Session: "s1", Status: "none", Embed: "ok", Rerank: "ok", TotalMs: 100},
		statefile.Turn{Time: sb.ago(time.Hour), Event: statefile.EventTool, Session: "s1", Tool: "Bash", Status: "inject", Keys: []string{"push-rule"}, Chars: 150,
			Fired: []statefile.Fired{{Key: "push-rule", Tool: "Bash", Regex: `git\s+push`}}},
		statefile.Turn{Time: sb.ago(2 * time.Hour), Event: statefile.EventTool, Session: "s2", Agent: "a1", Tool: "Bash", Status: "inject", Keys: []string{"push-rule"}, Chars: 150,
			Fired: []statefile.Fired{{Key: "push-rule", Tool: "Bash", Regex: `git\s+push`}}},
		statefile.Turn{Time: sb.ago(time.Hour), Event: statefile.EventTool, Session: "s1", Agent: "a2", Tool: "SubagentStart:explore", Status: "inject", Chars: 900},
		statefile.Turn{Time: sb.ago(10 * day), Event: statefile.EventTool, Session: "s3", Tool: "Edit", Status: "inject", Keys: []string{"old-trigger"}, Chars: 10},
	)
	log := strings.Join([]string{
		sb.ago(time.Hour) + " user-prompt-submit session=s1 inject 30ms hits 4; pointers over 2500 chars, left out deploy-state,long-state; keys push-rule=0.700",
		sb.ago(time.Hour) + " pre-tool-use session=s1 inject 12ms over 2500 chars, left out deploy-state",
		sb.ago(time.Hour) + " user-prompt-submit session=s1 none 100ms hits 0",
		sb.ago(9*day) + " pre-tool-use session=s3 inject 12ms over 2500 chars, left out ancient",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(sb.stateDir(), statefile.HookLogName), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	r := sb.stats(t)
	p := r.Prompts
	switch {
	case r.Days != 7 || r.Since != "2026-10-31T12:00:00Z" || r.Until != "2026-11-07T12:00:00Z":
		t.Errorf("window %d %q %q", r.Days, r.Since, r.Until)
	case p.Total != 3 || p.Skipped != 1 || p.Injected != 2 || p.None != 1 || p.Errors != 0 || p.WithText != 1 || p.PointersOnly != 1 || p.Chars != 480:
		t.Errorf("prompts %+v", p)
	case p.Embed["ok"] != 2 || p.Embed["down"] != 1 || p.Rerank["ok"] != 2 || p.Rerank["-"] != 1:
		t.Errorf("outcomes %v %v", p.Embed, p.Rerank)
	case p.TotalMs != stats.Latency{P50: 100, P90: 900, P99: 900, Max: 900}:
		t.Errorf("latency %+v", p.TotalMs)
	case r.Sessions != stats.Sessions{Starts: 1, Injected: 1, Chars: 2000}:
		t.Errorf("sessions %+v", r.Sessions)
	case r.Triggers.Injections != 2 || r.Triggers.Main != 1 || r.Triggers.Subagent != 1 || r.Triggers.Chars != 300 || len(r.Triggers.Keys) != 1:
		t.Errorf("triggers %+v", r.Triggers)
	case r.Triggers.Keys[0].Key != "push-rule" || r.Triggers.Keys[0].Fires != 2 || r.Triggers.Keys[0].Sessions != 2 || strings.Join(r.Triggers.Keys[0].Regex, " ") != `git\s+push`:
		t.Errorf("trigger key %+v", r.Triggers.Keys[0])
	case r.Subagents != stats.Subagents{Starts: 1, MapOnly: 1}:
		t.Errorf("subagents %+v", r.Subagents)
	case r.Dropped.Events != 2 || len(r.Dropped.Keys) != 2 || r.Dropped.Keys[0] != stats.DroppedKey{Key: "deploy-state", Times: 2}:
		t.Errorf("dropped %+v", r.Dropped)
	case r.State == nil || r.StateError != "":
		t.Fatalf("state missing: %q", r.StateError)
	case r.State.Current != 3 || r.State.Expired != 0 || r.State.Expiring7d != 1 || r.State.Expiring14d != 2 || r.State.NeverVerified != 3:
		t.Errorf("state %+v", r.State)
	case len(r.State.Soon) != 2 || r.State.Soon[0].Key != "deploy-state" || r.State.Soon[1].Key != "migration-state":
		t.Errorf("soon %+v", r.State.Soon)
	}

	cases := []struct {
		name                     string
		args                     []string
		total, injections, drops int
	}{
		{name: "everything", args: []string{"--days", "0"}, total: 4, injections: 3, drops: 3},
		{name: "two weeks", args: []string{"--days", "14"}, total: 3, injections: 3, drops: 3},
		{name: "one day", args: []string{"--days", "1"}, total: 2, injections: 2, drops: 2},
	}
	for _, c := range cases {
		r := sb.stats(t, c.args...)
		if r.Prompts.Total != c.total || r.Triggers.Injections != c.injections || r.Dropped.Events != c.drops {
			t.Errorf("%s: total %d injections %d drops %d", c.name, r.Prompts.Total, r.Triggers.Injections, r.Dropped.Events)
		}
	}
	if r := sb.stats(t, "--days", "0"); r.Since != "" || r.Days != 0 {
		t.Errorf("--days 0 window %q %d", r.Since, r.Days)
	}

	code, out, errs := sb.run("", "stats", "--top", "1")
	if code != 0 {
		t.Fatalf("stats text: %d %s", code, errs)
	}
	for _, w := range []string{
		"window 7 days, 2026-10-31T12:00:00Z to 2026-11-07T12:00:00Z\n",
		"prompts 3: injected 2 (text 1, pointers only 1), none 1, errors 0, skipped 1, chars 480\n",
		"embed: ok 2, down 1; degraded notices 0\n",
		"rerank: ok 2, - 1\n",
		"latency ms: p50 100, p90 900, p99 900, max 900\n",
		"sessions 1: injected 1, chars 2000\n",
		"triggers 2: main 1, subagent 1, chars 300, keys 1\n",
		"fires  sessions  key  regex\n    2         2  push-rule  git\\s+push\n",
		"subagents 1: with recall 0, map only 1, recall at the first tool call 0\n",
		"tool failures with pointers 0: pointers 0\n",
		"state 3 current: expired 0, expiring 7d 1, expiring 14d 2, never verified 3\n",
		"expiring soon:\n2026-11-10  deploy-state\n+1 more\n",
		"dropped 2 events, 2 keys\ntimes  key\n    2  deploy-state\n+1 more\n",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("text lacks %q:\n%s", w, out)
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimRight(l, " \t") != l || strings.Contains(l, "\x1b") {
			t.Errorf("trailing space or escape in %q", l)
		}
	}

	sb.now = sb.now.Add(5 * 24 * time.Hour)
	if r := sb.stats(t); r.Prompts.Total != 2 || r.State.Expiring7d != 2 || r.State.Soon[0].Expires != "2026-11-10" {
		t.Errorf("five days later: total %d state %+v", r.Prompts.Total, r.State)
	}
}

func TestStatsWithoutData(t *testing.T) {
	sb := newSandbox(t)
	r := sb.stats(t)
	if r.State != nil || !strings.Contains(r.StateError, "no memory database") || r.Prompts.Total != 0 || r.Dropped.Keys == nil || r.Triggers.Keys == nil {
		t.Errorf("empty: %+v", r)
	}
	sb.appendTurns(t, statefile.TurnsName,
		statefile.Turn{Time: sb.ago(time.Minute), Event: statefile.EventPrompt, Session: "s", Status: "none", Embed: "ok", TotalMs: 7})
	r = sb.stats(t)
	if r.State != nil || r.Prompts.Total != 1 || r.Prompts.None != 1 {
		t.Errorf("turns without a database: %+v", r)
	}
	code, out, errs := sb.run("", "stats")
	if code != 0 || !strings.Contains(out, "prompts 1: injected 0") || !strings.Contains(out, "state unavailable: no memory database at ") {
		t.Errorf("text without a database: %d\n%s%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(sb.dir, "data", env.DirName, env.DBName)); err == nil {
		t.Error("stats created a database")
	}
}

func TestStatsFlags(t *testing.T) {
	sb := newSandbox(t)
	cases := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "negative days", args: []string{"stats", "--days", "-1"}, code: 2, stderr: "must not be negative"},
		{name: "negative top", args: []string{"stats", "--top", "-3"}, code: 2, stderr: "must not be negative"},
		{name: "positional", args: []string{"stats", "extra"}, code: 2, stderr: `unexpected argument "extra"`},
		{name: "bad flag", args: []string{"stats", "--nope"}, code: 2, stderr: "example: mem stats [--days N] [--top N] [--json]"},
	}
	for _, c := range cases {
		code, _, errs := sb.run("", c.args...)
		if code != c.code || !strings.Contains(errs, c.stderr) {
			t.Errorf("%s: %d %q", c.name, code, errs)
		}
	}
	if code, out, _ := sb.run("", "help"); code != 0 || !strings.Contains(out, "  stats [--days N] [--top N] [--json]") {
		t.Errorf("help lacks stats: %d", code)
	}
}

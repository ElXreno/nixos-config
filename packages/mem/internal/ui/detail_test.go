package ui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"mem/internal/statefile"
	"mem/internal/store"
)

func ptr(v float64) *float64 { return &v }

func TestDetailCarriesEveryRecordField(t *testing.T) {
	f, dir := stateFixture(t)
	appendJSON(t, filepath.Join(dir, statefile.TurnsName),
		statefile.Turn{Time: at(0), Event: statefile.EventPrompt, Session: "s1", Prompt: "p", Status: "inject", Project: "legacy",
			Strong:   []statefile.Key{{Key: "feedback_alpha", Cos: 0.8, Rerank: ptr(0.995)}},
			Pointers: []statefile.Key{{Key: "reference_beta", Cos: 0.6, Rerank: ptr(0.92)}},
			Reranked: []statefile.Key{{Key: "reference_lonely", Cos: 0.55, Rerank: ptr(0.3)}}},
		statefile.Turn{Time: at(10 * time.Second), Event: statefile.EventTool, Session: "s1", Tool: "Bash", Status: "inject", Keys: []string{"feedback_alpha"},
			Input: "alpha now", Fired: []statefile.Fired{{Key: "feedback_alpha", Tool: "Bash", Regex: `alpha\b`}}},
		statefile.Turn{Time: at(20 * time.Second), Event: statefile.EventSession, Session: "s1", Status: "inject", Keys: []string{"feedback_alpha"}})
	var d detailOut
	f.json(t, "/api/detail?key=feedback_alpha", &d)
	r := d.Record
	if r.Key != "feedback_alpha" || r.Version != 2 || !r.Core || r.Kind != "rule" || r.Source != "" || r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() || r.Body == nil || !strings.Contains(*r.Body, "revised") {
		t.Fatalf("record = %+v", r)
	}
	if len(d.Triggers) != 1 || !d.Triggers[0].OK || d.Triggers[0].Tool != "Bash" || d.Triggers[0].Regex != `alpha\b` {
		t.Fatalf("triggers = %+v", d.Triggers)
	}
	if d.Topic.ID != "kernel" || d.Topic.Label != "Kernel work" {
		t.Fatalf("topic = %+v", d.Topic)
	}
	if len(d.History) != 1 || d.History[0].Version != 1 || !strings.Contains(d.History[0].Body, "and `") && !strings.Contains(d.History[0].Body, "and ") || d.History[0].Reason == "" {
		t.Fatalf("history = %+v", d.History)
	}
	var plus, minus int
	for _, l := range d.History[0].Diff {
		switch l.Op {
		case "+":
			plus++
		case "-":
			minus++
		}
	}
	if plus != 1 || minus != 1 {
		t.Fatalf("diff = %+v", d.History[0].Diff)
	}
	if d.Injected.Total != 3 || len(d.Injected.Last) != 3 {
		t.Fatalf("injected = %+v", d.Injected)
	}
	tiers := []string{d.Injected.Last[0].Tier, d.Injected.Last[1].Tier, d.Injected.Last[2].Tier}
	if !slices.Equal(tiers, []string{tierSession, tierTool, tierStrong}) {
		t.Fatalf("tiers = %v", tiers)
	}
	if d.Injected.Last[1].Regex != `alpha\b` || d.Injected.Last[1].Input != "alpha now" {
		t.Fatalf("tool injection = %+v", d.Injected.Last[1])
	}
	if s := d.Injected.Last[2]; s.Cos == nil || *s.Cos != 0.8 || s.Rerank == nil || *s.Rerank != 0.995 {
		t.Fatalf("strong injection = %+v", s)
	}
	var lone detailOut
	f.json(t, "/api/detail?key=reference_lonely", &lone)
	if lone.Injected.Total != 0 || lone.Injected.Candidates != 1 || lone.Injected.Last[0].Tier != tierCandidate || *lone.Injected.Last[0].Rerank != 0.3 {
		t.Fatalf("candidate = %+v", lone.Injected)
	}
	if lone.Vector.State != "unknown" {
		t.Fatalf("vector without a model = %+v", lone.Vector)
	}
}

func TestDetailLinksAreTypedAndBrokenOnesResolve(t *testing.T) {
	f := newFixture(t)
	var d detailOut
	f.json(t, "/api/detail?key=reference_gamma", &d)
	by := map[string]linkOut{}
	for _, l := range d.Links {
		by[l.Dst] = l
	}
	if l := by["reference_beta"]; l.Rel != "relates" || l.Dir != "out" || !l.InGraph || l.Other != "reference_beta" {
		t.Fatalf("relates = %+v", l)
	}
	if l := by["old-slug"]; !slices.Equal(l.Resolved, []string{"reference_beta"}) || l.How != resolveDerived || l.InGraph {
		t.Fatalf("derived = %+v", l)
	}
	if l := by["project"]; !slices.Equal(l.Resolved, []string{"project_big", "project_state"}) || l.How != resolvePrefix {
		t.Fatalf("prefix = %+v", l)
	}
	if l := by["MEMORY"]; !l.NonKey || len(l.Resolved) != 0 {
		t.Fatalf("non key = %+v", l)
	}
	if l := by["missing-key"]; l.NonKey || len(l.Resolved) != 0 || l.Rel != "unresolved" {
		t.Fatalf("still broken = %+v", l)
	}
	var beta detailOut
	f.json(t, "/api/detail?key=reference_beta", &beta)
	var in, note int
	for _, l := range beta.Links {
		if l.Dir == "in" {
			in++
		}
		if l.Note == "evidence quote" && l.Rel == "derived_from" {
			note++
		}
	}
	if in < 2 || note != 1 {
		t.Fatalf("beta links in %d notes %d: %+v", in, note, beta.Links)
	}
	var big detailOut
	f.json(t, "/api/detail?key=project_big", &big)
	if len(big.Record.Chunks) != 2 || len(big.Links) != 1 || big.Links[0].Dir != "out" {
		t.Fatalf("log group = %+v", big)
	}
	if rec := f.do(t, http.MethodGet, "/api/detail?key=nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/detail"); rec.Code != http.StatusBadRequest {
		t.Fatalf("no key = %d", rec.Code)
	}
}

func TestDetailBodyRefsResolveTokens(t *testing.T) {
	f := newFixture(t)
	put(t, f.st, putReq("reference_refs", "Refs", "See `old-slug` and [[old-slug]] and `reference_beta` and `zzzz_none` and `project_big`."))
	var d detailOut
	f.json(t, "/api/detail?key=reference_refs", &d)
	if r, ok := d.Refs["old-slug"]; !ok || !slices.Equal(r.Keys, []string{"reference_beta"}) || r.How != resolveDerived {
		t.Fatalf("refs = %+v", d.Refs)
	}
	if _, ok := d.Refs["reference_beta"]; ok {
		t.Fatalf("an existing key is listed as a reference to resolve: %+v", d.Refs)
	}
	if _, ok := d.Refs["zzzz_none"]; ok {
		t.Fatalf("an unresolvable token is listed: %+v", d.Refs)
	}
}

func TestLineDiff(t *testing.T) {
	d := lineDiff("a\nb\nc\nd\ne\nf\ng\nh\ni\n", "a\nb\nc\nd\nX\nf\ng\nh\ni\n")
	ops := ""
	for _, l := range d {
		ops += l.Op
	}
	if ops != "skip   -+   " && ops != " skip  -+  " && !strings.Contains(ops, "-+") {
		t.Fatalf("ops = %q: %+v", ops, d)
	}
	same := lineDiff("x\ny", "x\ny")
	if len(same) != 1 || same[0].Op != "skip" || same[0].N != 2 {
		t.Fatalf("identical = %+v", same)
	}
	added := lineDiff("", "one\ntwo")
	if len(added) != 2 || added[0].Op != "+" {
		t.Fatalf("added = %+v", added)
	}
	huge := strings.Repeat("line\n", 3000)
	if d := lineDiff(huge, huge+"x"); len(d) != 1 || d[0].Op != "~" {
		t.Fatalf("huge = %+v", d)
	}
}

func TestSurfacedOrphansApplyOnlyWithAFullWindow(t *testing.T) {
	f, dir := stateFixture(t)
	old := statefile.Turn{Time: tNow.Add(-40 * 24 * time.Hour).Format(time.RFC3339), Event: statefile.EventPrompt, Session: "s1", Prompt: "p", Status: "inject", Strong: []statefile.Key{{Key: "reference_gamma", Cos: 0.7}}}
	recent := statefile.Turn{Time: tNow.Add(-24 * time.Hour).Format(time.RFC3339), Event: statefile.EventTool, Session: "s1", Tool: "Bash", Status: "inject", Keys: []string{"project_big#2", "feedback_new"}}
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), recent)
	var h Hygiene
	f.json(t, "/api/hygiene", &h)
	if h.Surfaced.Applied || h.Surfaced.CoveredDays != 1 {
		t.Fatalf("one day of turns must not mark records unsurfaced: %+v", h.Surfaced)
	}
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), old)
	f.json(t, "/api/hygiene", &h)
	if !h.Surfaced.Applied || h.Surfaced.CoveredDays < 30 {
		t.Fatalf("surfaced = %+v", h.Surfaced)
	}
	why := map[string][]string{}
	for _, it := range section(h, "orphans").Items {
		why[it.Key] = it.Why
	}
	if !slices.Contains(why["reference_beta"], reasonUnsurface) || !slices.Contains(why["reference_lonely"], reasonUnsurface) || !slices.Contains(why["reference_lonely"], reasonNoTopic) {
		t.Fatalf("orphans = %v", why)
	}
	for _, k := range []string{"project_big", "feedback_new", "feedback_alpha"} {
		if slices.Contains(why[k], reasonUnsurface) {
			t.Fatalf("%s was surfaced or core but is marked unsurfaced: %v", k, why[k])
		}
	}
	if _, ok := why["feedback_old"]; ok {
		t.Fatal("a superseded record is an orphan")
	}
}

func TestTurnsKeepTheNewContextAndRerankFields(t *testing.T) {
	f, dir := stateFixture(t)
	appendJSON(t, filepath.Join(dir, statefile.TurnsName),
		statefile.Turn{Time: at(0), Event: statefile.EventPrompt, Session: "s1", Prompt: "p", Status: "inject", Harness: "pi", Host: "smok", Project: "a/b", Cwd: "~/x", AgentType: "Explore",
			Strong: []statefile.Key{{Key: "feedback_alpha", Cos: 0.8, Rerank: ptr(0.99)}}, Pointers: []statefile.Key{{Key: "reference_beta", Cos: 0.5, Rerank: ptr(0.91)}},
			Rerank: "ok", RerankMs: 120, Reranked: []statefile.Key{{Key: "reference_gamma", Cos: 0.4, Rerank: ptr(0.02)}}},
		statefile.Turn{Time: at(time.Second), Event: statefile.EventTool, Session: "s1", Tool: "Edit", Status: "inject", Keys: []string{"feedback_alpha"}, Input: "/a/b.go | edit it",
			Fired: []statefile.Fired{{Key: "feedback_alpha", Tool: "Edit", Regex: "x"}}},
		statefile.Turn{Time: at(2 * time.Second), Event: statefile.EventSession, Session: "s1", Status: "inject", Source: "startup", Keys: []string{"feedback_alpha"}, Chars: 500, Topics: 22},
		statefile.Turn{Time: at(3 * time.Second), Event: statefile.EventPrompt, Session: "s1", Prompt: "<task-notification>", Status: "skipped", Source: "<task-notification"},
		statefile.Turn{Time: at(4 * time.Second), Event: statefile.EventPrompt, Session: "s1", Prompt: "boom", Status: "error"})
	r := getTurns(t, f, nil)
	if len(r.Turns) != 5 {
		t.Fatalf("turns = %d", len(r.Turns))
	}
	p, tl, ss, sk, er := r.Turns[0], r.Turns[1], r.Turns[2], r.Turns[3], r.Turns[4]
	if p.Harness != "pi" || p.Host != "smok" || p.Project != "a/b" || p.Cwd != "~/x" || p.AgentType != "Explore" || p.Rerank != "ok" || p.RerankMs != 120 {
		t.Fatalf("context = %+v", p)
	}
	if p.Strong[0].Rerank == nil || *p.Strong[0].Rerank != 0.99 || p.Pointers[0].Rerank == nil || *p.Pointers[0].Rerank != 0.91 || len(p.Reranked) != 1 || *p.Reranked[0].Rerank != 0.02 {
		t.Fatalf("rerank scores were dropped: %+v", p)
	}
	if tl.Input != "/a/b.go | edit it" || len(tl.Fired) != 1 || tl.Fired[0].Regex != "x" {
		t.Fatalf("tool turn = %+v", tl)
	}
	if ss.Event != statefile.EventSession || ss.Topics != 22 || ss.Chars != 500 || ss.Source != "startup" || ss.Origin != "turns" {
		t.Fatalf("session turn = %+v", ss)
	}
	if sk.Status != "skipped" || sk.Source != "<task-notification" || er.Status != "error" {
		t.Fatalf("skipped/error = %+v %+v", sk, er)
	}
	var s struct {
		Sessions []sessionOut `json:"sessions"`
	}
	f.json(t, "/api/sessions", &s)
	if len(s.Sessions) != 1 || s.Sessions[0].Project != "" && s.Sessions[0].Harness == "" {
		t.Fatalf("sessions = %+v", s.Sessions)
	}
	if s.Sessions[0].Prompts != 3 || s.Sessions[0].Tools != 1 {
		t.Fatalf("session events were counted as prompts: %+v", s.Sessions[0])
	}
}

func TestHealthLatencyPercentilesComeFromTurns(t *testing.T) {
	f, dir := stateFixture(t)
	var ts []any
	for i := 1; i <= 100; i++ {
		ts = append(ts, statefile.Turn{Time: at(time.Duration(i) * time.Second), Event: statefile.EventPrompt, Session: "s", Prompt: "p", Status: "inject", Chars: 100, TotalMs: int64(i), Embed: "ok", EmbedMs: int64(i * 2)})
	}
	ts = append(ts, statefile.Turn{Time: tNow.Add(-time.Hour).Format(time.RFC3339), Event: statefile.EventPrompt, Session: "s", Status: "error"})
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), ts...)
	appendRaw(t, filepath.Join(dir, statefile.HookLogName), "2026-11-07T10:00:10Z user-prompt-submit session=s error 7000ms over the 7s budget\n")
	var h healthOut
	f.json(t, "/api/health", &h)
	l := h.Hook.Latency["total_ms"]
	if l.N != 100 || l.P50 < 50 || l.P50 > 51 || l.P90 < 90 || l.P90 > 91 || l.P99 < 99 || l.Max != 100 {
		t.Fatalf("total latency = %+v", l)
	}
	if e := h.Hook.Latency["embed_ms"]; e.N != 100 || e.Max != 200 {
		t.Fatalf("embed latency = %+v", e)
	}
	if h.Hook.Turns != 102 || h.Hook.Errors24h != 2 || h.Hook.InjectedShare != 1 || h.Hook.AvgChars != 100 {
		t.Fatalf("hook = %+v", h.Hook)
	}
	if len(h.Hook.LastErrors) != 1 || h.Hook.LastErrors[0].Event != "user-prompt-submit" || !strings.Contains(h.Hook.LastErrors[0].Message, "budget") {
		t.Fatalf("errors = %+v", h.Hook.LastErrors)
	}
	if h.Calibration.StrongCos.Counts == nil || len(h.Calibration.Rerank.Counts) != len(rerankEdges)-1 {
		t.Fatalf("calibration = %+v", h.Calibration)
	}
}

func TestHistogramBins(t *testing.T) {
	h := newHistogram(rerankEdges)
	for _, v := range []float64{0, 0.005, 0.02, 0.5, 0.95, 0.99, 1} {
		h.add(v)
	}
	want := []int{2, 1, 0, 0, 0, 1, 0, 1, 2}
	if !slices.Equal(h.Counts, want) {
		t.Fatalf("bins = %v, want %v", h.Counts, want)
	}
}

func putReq(key, title, body string) store.PutRequest {
	return store.PutRequest{Key: key, Kind: store.Fact, Title: sp(title), Body: sp(body)}
}

func TestDiffKeepsBlankLinesAsText(t *testing.T) {
	data, err := json.Marshal(lineDiff("a\n\nb\n", "a\n\nc\n"))
	if err != nil {
		t.Fatal(err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	blank := 0
	for _, l := range raw {
		if op := l["op"]; op == " " || op == "+" || op == "-" {
			text, ok := l["text"]
			if !ok {
				t.Fatalf("a %q line carries no text key: %s", op, data)
			}
			if text == "" {
				blank++
			}
		}
	}
	if blank == 0 {
		t.Fatalf("no blank line survived: %s", data)
	}
}

func TestCalibrationJoinsTheShadowLog(t *testing.T) {
	f, dir := stateFixture(t)
	const sha = "aaaaaaaaaaaaaaaa"
	turn := promptTurn("s1", sha, "first prompt", 0)
	turn.Strong = []statefile.Key{{Key: "feedback_alpha", Cos: 0.81}}
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), turn)
	appendJSON(t, filepath.Join(dir, statefile.ShadowName), shadowFix{Time: at(time.Second), Session: "s1", Prompt: sha, Pool: "fused-reg", Embed: "ok",
		Keys: []string{"feedback_alpha", "reference_net"}, Hits: []shadowHitFix{{Key: "feedback_alpha", Cos: 0.81}, {Key: "reference_net", Cos: 0.66}, {Key: "below_gate", Cos: 0.35}}})
	var h healthOut
	f.json(t, "/api/health", &h)
	if h.Calibration.Shadow != 1 || h.Calibration.ShadowOnly != 1 {
		t.Fatalf("shadow lines = %d, shadow only = %d", h.Calibration.Shadow, h.Calibration.ShadowOnly)
	}
	total := 0
	for _, c := range h.Calibration.ShadowCos.Counts {
		total += c
	}
	if total != 3 {
		t.Fatalf("shadow cosines binned = %d", total)
	}
	sweep := map[string]sweepRow{}
	for _, r := range h.Calibration.Sweep {
		sweep[strconv.FormatFloat(r.T, 'f', 2, 64)] = r
	}
	if r := sweep["0.80"]; r.Strong != 1 || r.Shadow != 1 {
		t.Fatalf("sweep at 0.80 = %+v", r)
	}
	if r := sweep["0.85"]; r.Strong != 0 || r.Shadow != 0 {
		t.Fatalf("sweep at 0.85 = %+v", r)
	}
	if r := sweep["0.65"]; r.Shadow != 2 {
		t.Fatalf("sweep at 0.65 = %+v", r)
	}
	if r := sweep["0.70"]; r.Shadow != 1 {
		t.Fatalf("sweep at 0.70 = %+v", r)
	}
	appendJSON(t, filepath.Join(dir, statefile.ShadowName), shadowFix{Time: at(2 * time.Second), Session: "s1", Prompt: sha, Pool: "fused-reg", Embed: "ok", Keys: []string{"more"}})
	f.json(t, "/api/health", &h)
	if h.Calibration.Shadow != 1 {
		t.Fatalf("a shadow line appended later is not picked up or is double counted: %d", h.Calibration.Shadow)
	}
}

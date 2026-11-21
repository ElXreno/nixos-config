package cli

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func axisVec(text string) []float64 {
	lower := strings.ToLower(text)
	v := make([]float64, 4)
	v[3] = 0.1
	for axis, words := range [][]string{{"push", "commits", "upstream"}, {"arc", "zfs"}, {"bluetooth"}} {
		for _, w := range words {
			if strings.Contains(lower, w) {
				v[axis] = 1
				break
			}
		}
	}
	var s float64
	for _, x := range v {
		s += x * x
	}
	for i := range v {
		v[i] /= math.Sqrt(s)
	}
	return v
}

func axisServer(t *testing.T, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad", 400)
			return
		}
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i, in := range req.Input {
			data = append(data, item{i, axisVec(in)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func seedForVectors(t *testing.T, sb *sandbox) {
	t.Helper()
	for _, p := range []struct{ key, kind, title, body string }{
		{"no-push-default", "rule", "No git push by default", "Do not git push by default. After a commit, wait for explicit authorization."},
		{"zfs-arc-ram", "fact", "ZFS ARC size", "The ARC grows to half of RAM when zfs_arc_max is unset."},
		{"bt-journal#1", "log", "", "2026-04-12 bluetooth headset pairing failed again on the tablet."},
	} {
		args := []string{"put", "--key", p.key, "--kind", p.kind, "--scope", "global"}
		if p.title != "" {
			args = append(args, "--title", p.title)
		}
		if code, _, errs := sb.run(p.body, args...); code != 0 {
			t.Fatalf("put %s: %d %s", p.key, code, errs)
		}
	}
}

func TestReindexAndVecStatus(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	reindex := []string{"reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: ", "--batch", "2", "--par", "2"}

	code, out, errs := sb.run("", "vec", "status")
	if code != 0 || !strings.Contains(out, "records 3 current 3") || !strings.Contains(out, "no vectors") {
		t.Errorf("status before: %d %q %q", code, out, errs)
	}
	code, out, errs = sb.run("", reindex...)
	if code != 0 || !strings.Contains(out, "model axis") || !strings.Contains(out, "embedded 3") || !strings.Contains(out, "dim 4") {
		t.Fatalf("reindex: %d %q %q", code, out, errs)
	}
	first := reqs.Load()
	code, out, _ = sb.run("", reindex...)
	if code != 0 || !strings.Contains(out, "embedded 0") || !strings.Contains(out, "fresh 3") || reqs.Load() != first {
		t.Errorf("second reindex: %d %q, %d requests", code, out, reqs.Load()-first)
	}
	code, out, _ = sb.run("", "vec", "status", "--model", "axis", "--doc-prefix", "Document: ")
	if code != 0 || !strings.Contains(out, "fresh 3 stale 0 missing 0") || !strings.Contains(out, "axis dim 4 rows 3") {
		t.Errorf("status: %d %q", code, out)
	}
	code, out, _ = sb.run("", "vec", "status", "--model", "axis", "--doc-prefix", "Passage: ")
	if code != 0 || !strings.Contains(out, "fresh 0 stale 3 missing 0") {
		t.Errorf("status with another prefix: %d %q", code, out)
	}
	if code, _, errs := sb.run("zfs arc is now capped by zfs_arc_max at 8 GB.", "put", "--key", "zfs-arc-ram", "--update"); code != 0 {
		t.Fatalf("update: %d %s", code, errs)
	}
	_, out, _ = sb.run("", "vec", "status", "--model", "axis", "--doc-prefix", "Document: ", "--json")
	var st struct {
		Fresh, Stale, Missing, Current int
		Models                         []struct {
			Model string
			Dim   int
			Rows  int
		}
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil || st.Fresh != 2 || st.Missing != 1 || st.Stale != 0 || st.Current != 3 || len(st.Models) != 1 {
		t.Errorf("json status: %v %+v\n%s", err, st, out)
	}
	code, out, _ = sb.run("", reindex...)
	if code != 0 || !strings.Contains(out, "embedded 1") {
		t.Errorf("reindex after the edit: %d %q", code, out)
	}
	code, out, _ = sb.run("", append(reindex, "--skip-logs", "--force")...)
	if code != 0 || !strings.Contains(out, "embedded 2") || !strings.Contains(out, "skipped logs 1") {
		t.Errorf("forced reindex without logs: %d %q", code, out)
	}
}

func TestReindexErrors(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	cases := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"no url", []string{"reindex", "--model", "m"}, 2, "--url"},
		{"no model", []string{"reindex", "--url", "http://x"}, 2, "--model"},
		{"bad batch", []string{"reindex", "--url", "http://x", "--model", "m", "--batch", "0"}, 2, "--batch"},
		{"server down", []string{"reindex", "--url", "http://127.0.0.1:1", "--model", "m", "--retries", "0"}, 1, "mem reindex"},
		{"unknown subcommand", []string{"vec", "nope"}, 2, "status"},
	}
	for _, c := range cases {
		code, out, errs := sb.run("", c.args...)
		if code != c.code || !strings.Contains(errs, c.stderr) || out != "" {
			t.Errorf("%s: exit %d stdout %q stderr %q", c.name, code, out, errs)
		}
	}
}

func TestReindexEnvironmentDefaults(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	sb.vars["MEM_EMBED_URL"] = srv.URL
	sb.vars["MEM_EMBED_MODEL"] = "axis"
	sb.vars["MEM_EMBED_DOC_PREFIX"] = "Document: "
	code, out, errs := sb.run("", "reindex")
	if code != 0 || !strings.Contains(out, "embedded 3") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	code, out, _ = sb.run("", "vec", "status")
	if code != 0 || !strings.Contains(out, "fresh 3 stale 0 missing 0") {
		t.Errorf("%d %q", code, out)
	}
}

func TestSearchVectorModes(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: "); code != 0 {
		t.Fatalf("%s", errs)
	}
	base := []string{"search", "--embed-url", srv.URL, "--embed-model", "axis", "--embed-query-prefix", "Query: "}
	for _, mode := range []string{"vector", "hybrid"} {
		code, out, errs := sb.run("", append(base, "--mode", mode, "should I send commits upstream")...)
		if code != 0 || !strings.HasPrefix(out, "1. no-push-default") {
			t.Errorf("%s: %d %q %q", mode, code, out, errs)
		}
	}
	code, out, _ := sb.run("", append(base, "--mode", "vector", "--kind", "log", "bluetooth headset")...)
	if code != 0 || !strings.Contains(out, "bt-journal#1") {
		t.Errorf("log kind: %d %q", code, out)
	}
	code, out, _ = sb.run("", append(base, "--mode", "vector", "--no-logs", "bluetooth headset")...)
	if code != 0 || strings.Contains(out, "bt-journal#1") {
		t.Errorf("no-logs: %d %q", code, out)
	}
	_, out, _ = sb.run("", append(base, "--mode", "vector", "--json", "should I send commits upstream")...)
	var hits []struct {
		Record struct{ Key string }
		Score  float64
	}
	if err := json.Unmarshal([]byte(out), &hits); err != nil || len(hits) == 0 || hits[0].Record.Key != "no-push-default" || hits[0].Score < 0.8 {
		t.Errorf("json: %v %s", err, out)
	}
	code, _, errs := sb.run("", "search", "--mode", "vector", "x y")
	if code != 2 || !strings.Contains(errs, "--embed-url") {
		t.Errorf("without a server: %d %q", code, errs)
	}
	code, _, errs = sb.run("", "search", "--embed-url", "http://127.0.0.1:1", "--embed-model", "axis", "--embed-timeout", "300ms", "--mode", "vector", "x y")
	if code != 1 || !strings.Contains(errs, "mem search") {
		t.Errorf("server down: %d %q", code, errs)
	}
}

func TestReindexMaxDocs(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	args := []string{"reindex", "--url", srv.URL, "--model", "axis", "--max-docs", "2"}
	code, out, errs := sb.run("", args...)
	if code != 0 || !strings.Contains(out, "embedded 2") || !strings.Contains(out, "deferred 1") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	code, out, _ = sb.run("", args...)
	if code != 0 || !strings.Contains(out, "embedded 1") || !strings.Contains(out, "deferred 0") {
		t.Errorf("%d %q", code, out)
	}
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--max-docs", "-1"); code != 2 || !strings.Contains(errs, "--max-docs") {
		t.Errorf("%d %q", code, errs)
	}
}

func TestVecPairs(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	for _, p := range []struct {
		key, kind, title, body string
		extra                  []string
	}{
		{"push-upstream", "fact", "Ask before commits go upstream", "Pushing to upstream needs a yes.", nil},
		{"zfs-arc-cap", "fact", "ZFS ARC size cap", "The ARC grows to half of RAM when zfs_arc_max is unset, cap it.", []string{"--distinct", "zfs-arc-ram", "--why", "test pair"}},
		{"bt-headset", "fact", "Bluetooth headset", "The bluetooth headset pairs only after a reset.", nil},
	} {
		args := append([]string{"put", "--key", p.key, "--kind", p.kind, "--scope", "global", "--title", p.title}, p.extra...)
		if code, _, errs := sb.run(p.body, args...); code != 0 {
			t.Fatalf("put %s: %d %s", p.key, code, errs)
		}
	}
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis"); code != 0 {
		t.Fatalf("reindex: %s", errs)
	}
	code, out, errs := sb.run("", "vec", "pairs", "--model", "axis")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 2 || lines[0] != "pairs with cosine >= 0.90: 2, flagged by the lexical gate: 1, past the gate: 1" ||
		!strings.HasPrefix(lines[1], "1.000  no-push-default  push-upstream  [rule/fact]  sim ") {
		t.Errorf("text: %d %q %q", code, out, errs)
	}
	sb.vars["MEM_EMBED_MODEL"] = "axis"
	_, out, _ = sb.run("", "vec", "pairs", "--all", "--json")
	var res struct {
		MinCos  float64 `json:"min_cos"`
		Model   string
		Records int
		Total   int
		Gated   int
		Past    int
		Pairs   []struct {
			A, B   string
			KindA  string `json:"kind_a"`
			Cos    float64
			Shared int
			Gate   bool
		}
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.MinCos != 0.9 || res.Model != "axis" || res.Records != 5 || res.Total != 2 || res.Gated != 1 || res.Past != 1 ||
		len(res.Pairs) != 2 || res.Pairs[0].A != "no-push-default" || res.Pairs[0].Gate || res.Pairs[0].KindA != "rule" ||
		res.Pairs[1].A != "zfs-arc-cap" || res.Pairs[1].B != "zfs-arc-ram" || !res.Pairs[1].Gate || res.Pairs[1].Shared < 2 || res.Pairs[1].Cos != 1 {
		t.Errorf("json: %v %+v\n%s", err, res, out)
	}
	code, out, _ = sb.run("", "vec", "pairs", "--all", "--limit", "1")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); code != 0 || len(lines) != 2 || !strings.Contains(lines[0], ": 2,") || !strings.Contains(lines[1], "no-push-default") {
		t.Errorf("limit: %d %q", code, out)
	}
	for _, c := range []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"unknown model", []string{"vec", "pairs", "--model", "other"}, 1, "no vectors of model other"},
		{"negative limit", []string{"vec", "pairs", "--limit", "-1"}, 2, "--limit"},
		{"extra argument", []string{"vec", "pairs", "x"}, 2, "unexpected"},
	} {
		code, out, errs := sb.run("", c.args...)
		if code != c.code || !strings.Contains(errs, c.stderr) || out != "" {
			t.Errorf("%s: exit %d stdout %q stderr %q", c.name, code, out, errs)
		}
	}
	delete(sb.vars, "MEM_EMBED_MODEL")
	if code, _, errs := sb.run("", "vec", "pairs"); code != 2 || !strings.Contains(errs, "--model") {
		t.Errorf("no model: %d %q", code, errs)
	}
}

func TestVecPairsMinCosLiesBetweenHalfAndOne(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis"); code != 0 {
		t.Fatalf("reindex: %s", errs)
	}
	for _, bad := range []string{"0.49", "0", "-1", "1.01", "NaN", "-Inf"} {
		code, out, errs := sb.run("", "vec", "pairs", "--model", "axis", "--min-cos", bad)
		if code != 2 || out != "" || !strings.Contains(errs, "--min-cos must lie between 0.5 and 1") {
			t.Errorf("--min-cos %s: exit %d stdout %q stderr %q", bad, code, out, errs)
		}
	}
	for _, good := range []string{"0.5", "1"} {
		if code, out, errs := sb.run("", "vec", "pairs", "--model", "axis", "--min-cos", good); code != 0 || !strings.HasPrefix(out, "pairs with cosine >= ") {
			t.Errorf("--min-cos %s: exit %d stdout %q stderr %q", good, code, out, errs)
		}
	}
}

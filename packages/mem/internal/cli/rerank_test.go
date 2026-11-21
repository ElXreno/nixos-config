package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

func rerankStub(t *testing.T, scores map[string]float64, requests *atomic.Int32, docs *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var req struct {
			Query     string   `json:"query"`
			Documents []string `json:"documents"`
		}
		if r.URL.Path != "/v1/rerank" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad", 400)
			return
		}
		docs.Store(req.Documents)
		type res struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		}
		var out []res
		for i, d := range req.Documents {
			s := 0.0
			for m, v := range scores {
				if strings.Contains(d, m) {
					s = v
				}
			}
			out = append(out, res{i, s})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchRerank(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs, rr atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: "); code != 0 {
		t.Fatalf("%s", errs)
	}
	var docs atomic.Value
	rsrv := rerankStub(t, map[string]float64{"half of RAM": 0.9987, "explicit authorization": 0.25, "bluetooth": 0.5}, &rr, &docs)
	base := []string{"search", "--embed-url", srv.URL, "--embed-model", "axis", "--embed-query-prefix", "Query: ", "--mode", "vector"}
	code, out, errs := sb.run("", append(base, "should I send commits upstream")...)
	if code != 0 || !strings.HasPrefix(out, "1. no-push-default") || rr.Load() != 0 {
		t.Fatalf("without --rerank: %d %q %q, %d rerank requests", code, out, errs, rr.Load())
	}
	code, out, errs = sb.run("", append(base, "--rerank", "--logs", "--rerank-url", rsrv.URL, "should I send commits upstream")...)
	if code != 0 || rr.Load() != 1 {
		t.Fatalf("%d %q %q, %d rerank requests", code, out, errs, rr.Load())
	}
	lines := strings.Split(out, "\n")
	if !strings.HasPrefix(lines[0], "1. zfs-arc-ram") || !strings.Contains(lines[0], "rerank 0.9987") || !strings.Contains(lines[0], "score ") {
		t.Errorf("rerank order or score missing:\n%s", out)
	}
	if !strings.Contains(out, "2. bt-journal#1") || !strings.Contains(out, "3. no-push-default") || !strings.Contains(out, "rerank 0.2500") {
		t.Errorf("order:\n%s", out)
	}
	got, _ := docs.Load().([]string)
	if len(got) != 3 || !strings.Contains(strings.Join(got, "|"), "No git push by default\nDo not git push") {
		t.Errorf("documents %q", got)
	}
	sb.vars["MEM_RERANK_URL"] = rsrv.URL
	_, out, _ = sb.run("", append(base, "--rerank", "--json", "--rerank-doc-chars", "5", "--rerank-candidates", "2", "--limit", "1", "should I send commits upstream")...)
	var hits []struct {
		Record struct{ Key string }
		Score  float64
		Rerank *float64
	}
	if err := json.Unmarshal([]byte(out), &hits); err != nil || len(hits) != 1 || hits[0].Rerank == nil {
		t.Fatalf("json: %v %s", err, out)
	}
	if got, _ := docs.Load().([]string); len(got) != 2 || got[0] != "No git push by default\nDo no" {
		t.Errorf("env url, 2 candidates of 5 body chars: %q", got)
	}
	code, out, _ = sb.run("", "search", "--rerank", "--rerank-url", rsrv.URL, "--mode", "words", "ARC")
	if code != 0 || !strings.Contains(out, "1. zfs-arc-ram") || !strings.Contains(out, "rerank 0.9987") {
		t.Errorf("lexical mode reranks too: %d %q", code, out)
	}
	delete(sb.vars, "MEM_RERANK_URL")
	code, _, errs = sb.run("", append(base, "--rerank", "x y")...)
	if code != 2 || !strings.Contains(errs, "--rerank-url") {
		t.Errorf("no url: %d %q", code, errs)
	}
	code, _, errs = sb.run("", append(base, "--rerank", "--rerank-url", "http://127.0.0.1:1", "should I send commits upstream")...)
	if code != 1 || !strings.Contains(errs, "mem search: rerank (down)") {
		t.Errorf("reranker down: %d %q", code, errs)
	}
	code, _, errs = sb.run("", append(base, "--rerank", "--rerank-url", rsrv.URL, "--rerank-candidates", "0", "x y")...)
	if code != 2 || !strings.Contains(errs, "--rerank-candidates") {
		t.Errorf("bad candidates: %d %q", code, errs)
	}
}

func TestSearchRerankKeepsHitsPastCandidatesInFusedOrder(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs, rr atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: "); code != 0 {
		t.Fatalf("%s", errs)
	}
	var docs atomic.Value
	rsrv := rerankStub(t, map[string]float64{"half of RAM": 0.9987}, &rr, &docs)
	base := []string{"search", "--embed-url", srv.URL, "--embed-model", "axis", "--embed-query-prefix", "Query: ", "--mode", "vector", "--logs"}
	numbered := regexp.MustCompile(`(?m)^\d+\. (\S+) .*$`)
	_, plain, _ := sb.run("", append(base, "--limit", "10", "should I send commits upstream")...)
	fused := numbered.FindAllStringSubmatch(plain, -1)
	if len(fused) != 3 {
		t.Fatalf("without rerank: %d hits, want 3:\n%s", len(fused), plain)
	}
	code, out, errs := sb.run("", append(base, "--limit", "10", "--rerank", "--rerank-url", rsrv.URL, "--rerank-candidates", "2", "should I send commits upstream")...)
	if code != 0 || errs != "" {
		t.Fatalf("%d %q", code, errs)
	}
	got := numbered.FindAllStringSubmatch(out, -1)
	if len(got) != 3 || got[2][1] != fused[2][1] || !strings.Contains(got[0][0], "rerank ") || !strings.Contains(got[1][0], "rerank ") || strings.Contains(got[2][0], "rerank ") {
		t.Errorf("--rerank-candidates 2 of 3 hits: the third must follow unreranked\nfused:\n%s\nreranked:\n%s", plain, out)
	}
	_, out, _ = sb.run("", append(base, "--limit", "2", "--rerank", "--rerank-url", rsrv.URL, "--rerank-candidates", "1", "should I send commits upstream")...)
	got = numbered.FindAllStringSubmatch(out, -1)
	if len(got) != 2 || got[0][1] != fused[0][1] || got[1][1] != fused[1][1] || !strings.Contains(got[0][0], "rerank ") || strings.Contains(got[1][0], "rerank ") {
		t.Errorf("--rerank-candidates 1, --limit 2: the reranked hit, then the next fused hit, cut to the limit:\n%s", out)
	}
	_, out, _ = sb.run("", append(base, "--limit", "0", "--rerank", "--rerank-url", rsrv.URL, "--rerank-candidates", "1", "should I send commits upstream")...)
	if got = numbered.FindAllStringSubmatch(out, -1); len(got) != 3 {
		t.Errorf("--limit 0 means the default limit, with or without rerank: %d hits, want 3:\n%s", len(got), out)
	}
}

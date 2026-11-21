package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mem/internal/store"
)

const fakeModel = "fake-embed"

func fakeVec(text string) []float64 {
	t := strings.ToLower(text)
	v := []float64{0.01, 0.01, 0.01}
	for i, w := range []string{"building", "readahead", "nvidia"} {
		v[i] += float64(strings.Count(t, w))
	}
	return v
}

type remotes struct {
	embedCalls, rerankCalls, healthCalls atomic.Int32
	embedStatus, rerankStatus            atomic.Int32
	embedURL, rerankURL                  string
}

func newRemotes(t *testing.T) *remotes {
	t.Helper()
	r := &remotes{}
	emb := http.NewServeMux()
	emb.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { r.healthCalls.Add(1) })
	emb.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, req *http.Request) {
		r.embedCalls.Add(1)
		if st := int(r.embedStatus.Load()); st != 0 {
			w.WriteHeader(st)
			return
		}
		var in struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(req.Body).Decode(&in)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i, s := range in.Input {
			data = append(data, item{Index: i, Embedding: fakeVec(s)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	es := httptest.NewServer(emb)
	t.Cleanup(es.Close)
	rr := http.NewServeMux()
	rr.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {})
	rr.HandleFunc("/v1/rerank", func(w http.ResponseWriter, req *http.Request) {
		r.rerankCalls.Add(1)
		if st := int(r.rerankStatus.Load()); st != 0 {
			w.WriteHeader(st)
			return
		}
		var in struct {
			Query     string   `json:"query"`
			Documents []string `json:"documents"`
		}
		_ = json.NewDecoder(req.Body).Decode(&in)
		type res struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		}
		var out []res
		for i, d := range in.Documents {
			score := 0.1
			if strings.Contains(d, "Beta") {
				score = 0.97
			}
			out = append(out, res{i, score})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	})
	rs := httptest.NewServer(rr)
	t.Cleanup(rs.Close)
	r.embedURL, r.rerankURL = es.URL, rs.URL
	return r
}

func remoteFixture(t *testing.T) (*fixture, *remotes) {
	t.Helper()
	f := newFixture(t)
	rm := newRemotes(t)
	prefix := "Document: "
	recs, err := f.st.List(t.Context(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var rows []store.VecRow
	for _, rec := range recs {
		v := fakeVec(store.DocText(prefix, rec))
		fv := make([]float32, len(v))
		for i, x := range v {
			fv[i] = float32(x)
		}
		rows = append(rows, store.VecRow{Key: rec.Key, Vec: fv, TextSHA: store.TextSHA(store.DocText(prefix, rec))})
	}
	if _, _, err := f.st.PutVectors(t.Context(), fakeModel, prefix, rows); err != nil {
		t.Fatal(err)
	}
	f.srv = New(f.st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }, EmbedURL: rm.embedURL, EmbedModel: fakeModel, RerankURL: rm.rerankURL}, false)
	f.h = f.srv.Handler()
	return f, rm
}

type searchResp struct {
	Mode     string            `json:"mode"`
	Asked    string            `json:"asked"`
	Reranked bool              `json:"reranked"`
	Degraded map[string]string `json:"degraded"`
	Hits     []hitOut          `json:"hits"`
}

func hitKeys(hs []hitOut) []string {
	out := []string{}
	for _, h := range hs {
		out = append(out, h.Key)
	}
	return out
}

func TestSearchLexicalModesAndFilters(t *testing.T) {
	f := newFixture(t)
	var out searchResp
	f.json(t, "/api/search?q=nvidia+driver", &out)
	if out.Mode != "fused" || len(out.Hits) != 1 || out.Hits[0].Key != "project_big" || out.Hits[0].Hit != "project_big#2" || out.Hits[0].Score <= 0 || out.Hits[0].WordsRank != 1 || out.Hits[0].WordsBM25 == 0 {
		t.Fatalf("hits = %+v", out)
	}
	f.json(t, "/api/search?q=building&status=current", &out)
	if !slices.Equal(hitKeys(out.Hits), []string{"feedback_new", "reference_beta"}) {
		t.Fatalf("current-only hits = %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=building", &out)
	if len(out.Hits) != 3 || !slices.Contains(hitKeys(out.Hits), "feedback_old") {
		t.Fatalf("all-status hits = %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=building&status=superseded", &out)
	if !slices.Equal(hitKeys(out.Hits), []string{"feedback_old"}) {
		t.Fatalf("superseded hits = %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=building&kind=fact", &out)
	if !slices.Equal(hitKeys(out.Hits), []string{"reference_beta"}) {
		t.Fatalf("kind filter = %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=readahead&logs=0", &out)
	if len(out.Hits) != 0 {
		t.Fatalf("logs=0 still returns logs: %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=readahead&logs=1&project=legacy", &out)
	if !slices.Equal(hitKeys(out.Hits), []string{"project_big"}) {
		t.Fatalf("project filter = %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=readahead&project=other", &out)
	if len(out.Hits) != 0 {
		t.Fatalf("a foreign project matched: %v", hitKeys(out.Hits))
	}
	f.json(t, "/api/search?q=building&mode=words", &out)
	if out.Mode != "words" || len(out.Hits) == 0 || out.Hits[0].TriRank != 0 || out.Hits[0].WordsRank == 0 {
		t.Fatalf("words mode = %+v", out)
	}
	f.json(t, "/api/search?q=building&mode=trigram", &out)
	if out.Mode != "trigram" || len(out.Hits) == 0 || out.Hits[0].TriRank == 0 {
		t.Fatalf("trigram mode = %+v", out)
	}
	for _, bad := range []string{"/api/search", "/api/search?q=", "/api/search?q=" + strings.Repeat("a", maxQueryLen+1), "/api/search?q=x&limit=0", "/api/search?q=x&kind=zzz", "/api/search?q=x&mode=zzz", "/api/search?q=x&status=zzz", "/api/search?q=x&rn=0"} {
		if rec := f.do(t, http.MethodGet, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d", bad[:min(len(bad), 40)], rec.Code)
		}
	}
}

func TestSearchVectorAndHybridUseTheEmbedder(t *testing.T) {
	f, rm := remoteFixture(t)
	var out searchResp
	f.json(t, "/api/search?q=building&mode=vector&status=current", &out)
	if out.Mode != "vector" || out.Degraded != nil || len(out.Hits) == 0 {
		t.Fatalf("vector = %+v", out)
	}
	if out.Hits[0].Cos == nil || *out.Hits[0].Cos < 0.9 || out.Hits[0].Score != *out.Hits[0].Cos {
		t.Fatalf("the top vector hit carries no real cosine: %+v", out.Hits[0])
	}
	for i := 1; i < len(out.Hits); i++ {
		if *out.Hits[i].Cos > *out.Hits[i-1].Cos {
			t.Fatalf("vector hits are not ordered by cosine: %+v", out.Hits)
		}
	}
	for _, h := range out.Hits {
		if h.Status != "current" {
			t.Fatalf("vector search ignored the status filter: %+v", h)
		}
	}
	f.json(t, "/api/search?q=building&mode=hybrid&status=current", &out)
	if out.Mode != "hybrid" || len(out.Hits) == 0 || out.Hits[0].Cos == nil {
		t.Fatalf("hybrid = %+v", out)
	}
	if !slices.ContainsFunc(out.Hits, func(h hitOut) bool { return h.WordsRank > 0 && h.Cos != nil }) {
		t.Fatalf("hybrid carries no words rank beside the cosine: %+v", out.Hits)
	}
	for i, h := range out.Hits {
		if h.Score <= 0 || h.VecRank == 0 && h.WordsRank == 0 && h.TriRank == 0 {
			t.Fatalf("hybrid hit %d carries no fused score or rank: %+v", i, h)
		}
		if i > 0 && h.Score > out.Hits[i-1].Score {
			t.Fatalf("hybrid hits are not ordered by the fused score: %+v", out.Hits)
		}
	}
	f.json(t, "/api/search?q=readahead&mode=vector&logs=0&status=current", &out)
	for _, h := range out.Hits {
		if h.Kind == "log" {
			t.Fatalf("logs=0 returned %+v", h)
		}
	}
	if rm.embedCalls.Load() != 3 {
		t.Fatalf("embedder calls = %d", rm.embedCalls.Load())
	}
	before := rm.embedCalls.Load()
	f.json(t, "/api/search?q=building", &out)
	f.json(t, "/api/search?q=building&mode=words", &out)
	if rm.embedCalls.Load() != before || rm.rerankCalls.Load() != 0 {
		t.Fatal("a lexical search called a remote server")
	}
}

func TestSearchFallsBackWhenTheEmbedderFails(t *testing.T) {
	f, rm := remoteFixture(t)
	var out searchResp
	rm.embedStatus.Store(500)
	out = searchResp{}
	f.json(t, "/api/search?q=building&mode=vector", &out)
	if out.Mode != "fused" || out.Asked != "vector" || out.Degraded["embed"] != "http" || len(out.Hits) == 0 {
		t.Fatalf("fallback = %+v", out)
	}
	if out.Hits[0].Cos != nil {
		t.Fatalf("a fallback hit claims a cosine: %+v", out.Hits[0])
	}
	plain := New(f.st, Options{Now: func() time.Time { return tNow }}, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=building&mode=hybrid&rerank=1", nil)
	req.Host = "127.0.0.1:1"
	plain.Handler().ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Mode != "fused" || out.Degraded["embed"] != errNotConfigure || out.Degraded["rerank"] != errNotConfigure || out.Reranked {
		t.Fatalf("not configured = %+v", out)
	}
}

func TestSearchRerankIsOptInAndKeepsRealScores(t *testing.T) {
	f, rm := remoteFixture(t)
	var out searchResp
	f.json(t, "/api/search?q=building&mode=words&status=current&rerank=1", &out)
	if !out.Reranked || out.Degraded != nil || len(out.Hits) < 2 {
		t.Fatalf("rerank = %+v", out)
	}
	if out.Hits[0].Key != "reference_beta" || out.Hits[0].Rerank == nil || *out.Hits[0].Rerank != 0.97 || *out.Hits[1].Rerank != 0.1 {
		t.Fatalf("rerank scores or order: %+v", out.Hits)
	}
	if rm.rerankCalls.Load() != 1 {
		t.Fatalf("rerank calls = %d", rm.rerankCalls.Load())
	}
	rm.rerankStatus.Store(503)
	out = searchResp{}
	f.json(t, "/api/search?q=building&mode=words&status=current&rerank=1", &out)
	if out.Reranked || out.Degraded["rerank"] != "http" || len(out.Hits) == 0 || out.Hits[0].Rerank != nil {
		t.Fatalf("rerank failure = %+v", out)
	}
}

func TestSearchRemoteCallsAreBounded(t *testing.T) {
	f, _ := remoteFixture(t)
	for range maxRemoteCalls {
		if !f.srv.acquire() {
			t.Fatal("gate closed too early")
		}
	}
	if rec := f.do(t, http.MethodGet, "/api/search?q=building&mode=vector"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a busy gate = %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/search?q=building"); rec.Code != http.StatusOK {
		t.Fatalf("a lexical search was blocked: %d", rec.Code)
	}
	f.srv.release()
	if rec := f.do(t, http.MethodGet, "/api/search?q=building&mode=vector"); rec.Code != http.StatusOK {
		t.Fatalf("after a release = %d", rec.Code)
	}
}

func TestHealthProbesAndCounters(t *testing.T) {
	f, rm := remoteFixture(t)
	var h healthOut
	f.json(t, "/api/health", &h)
	if !h.Embedder.Configured || !h.Embedder.OK || !h.Embedder.Reachable || h.Embedder.Status != 200 || !h.Reranker.OK {
		t.Fatalf("services = %+v %+v", h.Embedder, h.Reranker)
	}
	if strings.Contains(h.Embedder.URL, "/health") || h.Embedder.URL == "" {
		t.Fatalf("url = %q", h.Embedder.URL)
	}
	f.json(t, "/api/health", &h)
	if rm.healthCalls.Load() != 1 {
		t.Fatalf("health is probed %d times within the cache window", rm.healthCalls.Load())
	}
	if h.Vectors.Model != fakeModel || h.Vectors.Fresh != 9 || h.Vectors.Missing != 0 || h.Vectors.Current != 9 || h.Vectors.Dim != 3 {
		t.Fatalf("vectors = %+v", h.Vectors)
	}
	if h.Gates["rerank_text"] != 0.99 || h.Gates["rerank_pointer"] != 0.9 {
		t.Fatalf("gates = %v", h.Gates)
	}
	plain := New(f.st, Options{Now: func() time.Time { return tNow }, EmbedURL: "http://127.0.0.1:1"}, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "127.0.0.1:1"
	plain.Handler().ServeHTTP(rec, req)
	var down healthOut
	if err := json.Unmarshal(rec.Body.Bytes(), &down); err != nil {
		t.Fatal(err)
	}
	if down.Embedder.Reachable || down.Embedder.OK || down.Embedder.Error == "" || down.Reranker.Configured {
		t.Fatalf("a dead embedder reads as healthy: %+v %+v", down.Embedder, down.Reranker)
	}
}

func TestCore(t *testing.T) {
	f := newFixture(t)
	var out struct {
		Budget  int       `json:"budget"`
		Size    int       `json:"size"`
		Over    bool      `json:"over"`
		Records []coreRow `json:"records"`
	}
	f.json(t, "/api/core", &out)
	if out.Budget != store.CoreBudget || len(out.Records) != 1 || out.Records[0].Key != "feedback_alpha" || out.Records[0].Chars <= 0 || out.Over {
		t.Fatalf("core = %+v", out)
	}
	if out.Records[0].InFile {
		t.Fatalf("a project-scoped record is in the core file: %+v", out.Records[0])
	}
	f.json(t, "/api/core?project=legacy&host=smok", &out)
	if len(out.Records) != 1 {
		t.Fatalf("core at project legacy = %+v", out)
	}
	f.json(t, "/api/core?project=other", &out)
	if len(out.Records) != 0 {
		t.Fatalf("core at a foreign project = %+v", out)
	}
}

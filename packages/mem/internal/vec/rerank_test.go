package vec

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

func rerankServer(t *testing.T, reply func(req rerankRequest) any) (*httptest.Server, *[]rerankRequest) {
	t.Helper()
	var seen []rerankRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/rerank" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "wrong route", 404)
			return
		}
		var req rerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		seen = append(seen, req)
		_ = json.NewEncoder(w).Encode(reply(req))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

type result struct {
	Index int     `json:"index"`
	Score float64 `json:"relevance_score"`
}

func sortedReply(scores []float64) any {
	var out []result
	for i, s := range scores {
		out = append(out, result{i, s})
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].Score > out[i].Score {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return map[string]any{"model": "qwen3-reranker", "object": "list", "results": out}
}

func TestRerankReturnsScoresByPosition(t *testing.T) {
	want := []float64{0.2, 0.999, 0, 1, 0.5}
	srv, seen := rerankServer(t, func(rerankRequest) any { return sortedReply(want) })
	docs := []string{"a", "b", "c", "d", "e"}
	got, err := NewReranker(RerankOptions{URL: srv.URL + "/", Timeout: time.Second}).Rerank(context.Background(), "the query", docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%d scores", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("score %d = %v, want %v", i, got[i], want[i])
		}
	}
	if len(*seen) != 1 {
		t.Fatalf("%d requests", len(*seen))
	}
	req := (*seen)[0]
	if req.Query != "the query" || strings.Join(req.Documents, ",") != "a,b,c,d,e" || req.TopN != 5 {
		t.Errorf("request %+v", req)
	}
}

func TestRerankNoDocumentsSendsNothing(t *testing.T) {
	srv, seen := rerankServer(t, func(rerankRequest) any { return sortedReply(nil) })
	got, err := NewReranker(RerankOptions{URL: srv.URL}).Rerank(context.Background(), "q", nil)
	if err != nil || len(got) != 0 || len(*seen) != 0 {
		t.Errorf("%v %v, %d requests", got, err, len(*seen))
	}
}

func rawRerank(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func hijack(fn func(rw *bufio.ReadWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		c, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		fn(rw)
	}
}

func TestRerankRejectsGarbage(t *testing.T) {
	ok3 := func(body string) http.HandlerFunc { return rawRerank(200, body) }
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"html page", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>captive portal</html>")) }, FailResponse},
		{"empty body", ok3(""), FailResponse},
		{"json null", ok3("null"), FailResponse},
		{"json array", ok3("[1,2,3]"), FailResponse},
		{"no results", ok3(`{"results":[]}`), FailResponse},
		{"results missing", ok3(`{"data":[]}`), FailResponse},
		{"results is not a list", ok3(`{"results":"nope"}`), FailResponse},
		{"too few results", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5}]}`), FailResponse},
		{"too many results", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":0.5},{"index":2,"relevance_score":0.5}]}`), FailResponse},
		{"index out of range", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":3,"relevance_score":0.5}]}`), FailResponse},
		{"negative index", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":-1,"relevance_score":0.5}]}`), FailResponse},
		{"repeated index", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":1,"relevance_score":0.5}]}`), FailResponse},
		{"index missing", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"relevance_score":0.5}]}`), FailResponse},
		{"index is a string", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":"2","relevance_score":0.5}]}`), FailResponse},
		{"score missing", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2}]}`), FailResponse},
		{"score null", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":null}]}`), FailResponse},
		{"score is a string", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":"0.9"}]}`), FailResponse},
		{"score above one", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":1.0001}]}`), FailResponse},
		{"score below zero", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":-0.0001}]}`), FailResponse},
		{"raw logit", ok3(`{"results":[{"index":0,"relevance_score":7.5},{"index":1,"relevance_score":-3.2},{"index":2,"relevance_score":0.1}]}`), FailResponse},
		{"number out of float64 range", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":1e999}]}`), FailResponse},
		{"NaN literal", ok3(`{"results":[{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.5},{"index":2,"relevance_score":NaN}]}`), FailResponse},
		{"truncated json", ok3(`{"results":[{"index":0,"relevance_score":0.5},`), FailResponse},
		{"http 500", rawRerank(500, `{"error":{"code":500,"message":"input is too large to process. increase the physical batch size"}}`), FailHTTP},
		{"http 503", rawRerank(503, `{"error":{"message":"Loading model"}}`), FailHTTP},
		{"http 404", rawRerank(404, "File Not Found"), FailHTTP},
		{"http 501 rerank off", rawRerank(501, `{"error":{"message":"This server does not support reranking. Start it with --reranking"}}`), FailHTTP},
		{"connection closed at once", hijack(func(*bufio.ReadWriter) {}), FailResponse},
		{"half a status line", hijack(func(rw *bufio.ReadWriter) { _, _ = rw.WriteString("HTTP/1.1 200 O"); _ = rw.Flush() }), FailResponse},
		{"never answers", hijack(func(*bufio.ReadWriter) { time.Sleep(2 * time.Second) }), FailTimeout},
		{"stalled body", hijack(func(rw *bufio.ReadWriter) {
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 400\r\n\r\n{\"results\":[")
			_ = rw.Flush()
			time.Sleep(2 * time.Second)
		}), FailTimeout},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			start := time.Now()
			got, err := NewReranker(RerankOptions{URL: srv.URL, Timeout: 300 * time.Millisecond}).Rerank(context.Background(), "q", []string{"a", "b", "c"})
			if err == nil {
				t.Fatalf("accepted %v", got)
			}
			if o := Classify(err); o != c.want {
				t.Errorf("classified %q (%v), want %q", o, err, c.want)
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v", d)
			}
		})
	}
}

func TestRerankDownAndCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	_, err = NewReranker(RerankOptions{URL: "http://" + addr, Timeout: time.Second}).Rerank(context.Background(), "q", []string{"a"})
	if o := Classify(err); o != FailDown {
		t.Errorf("refused port classified %q (%v)", o, err)
	}
	slow := httptest.NewServer(http.HandlerFunc(hang))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = NewReranker(RerankOptions{URL: slow.URL, Timeout: 10 * time.Second}).Rerank(ctx, "q", []string{"a"})
	if o := Classify(err); o != FailTimeout || time.Since(start) > time.Second {
		t.Errorf("context deadline: %q (%v) after %v", o, err, time.Since(start))
	}
}

func TestRerankCapsTheReply(t *testing.T) {
	for _, status := range []int{200, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			chunk := []byte(strings.Repeat("a", 1<<14))
			for {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		}))
		start := time.Now()
		_, err := NewReranker(RerankOptions{URL: srv.URL, MaxBody: 1 << 16, Timeout: 5 * time.Second}).Rerank(context.Background(), "q", []string{"a"})
		srv.Close()
		want := map[int]string{200: FailResponse, 500: FailHTTP}[status]
		if o := Classify(err); err == nil || o != want {
			t.Errorf("status %d: %q (%v), want %q", status, o, err, want)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("status %d took %v", status, d)
		}
	}
}

func TestRerankErrorWithholdsAnEchoedQuery(t *testing.T) {
	const query = "this prompt text must never reach a log line"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rerankRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		http.Error(w, "cannot rank: "+req.Query, 400)
	}))
	defer srv.Close()
	_, err := NewReranker(RerankOptions{URL: srv.URL}).Rerank(context.Background(), query, []string{"a"})
	if err == nil || strings.Contains(err.Error(), "must never reach") || Classify(err) != FailHTTP {
		t.Errorf("error %v", err)
	}
}

func TestCheckScores(t *testing.T) {
	cases := []struct {
		name   string
		scores []float64
		n      int
		ok     bool
	}{
		{"boundaries", []float64{0, 1, 0.5}, 3, true},
		{"empty", nil, 0, true},
		{"NaN", []float64{0.5, math.NaN()}, 2, false},
		{"plus infinity", []float64{math.Inf(1)}, 1, false},
		{"minus infinity", []float64{math.Inf(-1)}, 1, false},
		{"above one", []float64{math.Nextafter(1, 2)}, 1, false},
		{"below zero", []float64{math.Nextafter(0, -1)}, 1, false},
		{"too few", []float64{0.5}, 2, false},
		{"too many", []float64{0.5, 0.5, 0.5}, 2, false},
	}
	for _, c := range cases {
		err := CheckScores(c.scores, c.n)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
		if err != nil && (!errors.Is(err, ErrBadRerank) || Classify(err) != FailResponse) {
			t.Errorf("%s: %v classified %q", c.name, err, Classify(err))
		}
	}
}

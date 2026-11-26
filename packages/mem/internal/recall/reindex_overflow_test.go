package recall

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"mem/internal/store"
	"mem/internal/vec"
)

func TestReindexClipsARecordOverTheEmbedderContext(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	recs := []store.Record{
		{Key: "huge", Kind: store.Fact, Scopes: []string{"global"}, Title: "Huge", Body: strings.Repeat("шум ", 3000)},
		{Key: "small-a", Kind: store.Fact, Scopes: []string{"global"}, Title: "A", Body: "short"},
		{Key: "small-b", Kind: store.Fact, Scopes: []string{"global"}, Title: "B", Body: "short too"},
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	const nCtx = 1000
	ref := &fakeEmbedder{}
	var mu sync.Mutex
	var seen []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for _, in := range req.Input {
			n := utf8.RuneCountInString(in) / 4
			mu.Lock()
			seen = append(seen, n)
			mu.Unlock()
			if n > nCtx {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": 400, "type": "exceed_context_size_error", "n_prompt_tokens": n, "n_ctx": nCtx,
					"message": "request exceeds the available context size",
				}})
				return
			}
		}
		vs, _ := ref.Embed(ctx, req.Input)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		for i, v := range vs {
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	var progress bytes.Buffer
	o := opts(vec.NewClient(vec.Options{URL: srv.URL, Model: "m", Timeout: 5 * time.Second}))
	o.Batch, o.Par, o.Progress = 16, 1, &progress
	res, err := Reindex(ctx, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 3 || !slices.Equal(res.Clipped, []string{"huge"}) {
		t.Fatalf("embedded %d, clipped %v", res.Embedded, res.Clipped)
	}
	if !strings.Contains(progress.String(), "clipped huge to ") {
		t.Errorf("progress does not name the clipped record: %q", progress.String())
	}
	if last := seen[len(seen)-1]; last > nCtx {
		t.Errorf("last request was %d tokens", last)
	}
	res, err = Reindex(ctx, s, o)
	if err != nil || res.Embedded != 0 || res.Fresh != 3 {
		t.Fatalf("second run embedded %d, fresh %d, %v", res.Embedded, res.Fresh, err)
	}
}

func TestReindexStillFailsOnOtherClientErrors(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":400,"type":"invalid_request_error"}}`, 400)
	}))
	defer srv.Close()
	o := opts(vec.NewClient(vec.Options{URL: srv.URL, Model: "m", Timeout: 5 * time.Second}))
	if _, err := Reindex(ctx, s, o); err == nil || !strings.Contains(err.Error(), "http 400") {
		t.Fatalf("got %v", err)
	}
}

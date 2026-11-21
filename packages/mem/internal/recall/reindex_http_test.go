package recall

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mem/internal/store"
	"mem/internal/vec"
)

func TestReindexKeepsEachVectorWithItsRecordWhenTheServerShufflesTheReply(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	ref := &fakeEmbedder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		vs, _ := ref.Embed(ctx, req.Input)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		for i := len(vs) - 1; i >= 0; i-- {
			data = append(data, item{i, vs[i]})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	cl := vec.NewClient(vec.Options{URL: srv.URL, Model: "m", Timeout: 5 * time.Second})
	o := opts(cl)
	o.Batch, o.Par = 3, 2
	if _, err := Reindex(ctx, s, o); err != nil {
		t.Fatal(err)
	}
	recs, err := s.List(ctx, store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	truth := &fakeEmbedder{}
	seen := 0
	err = s.ScanVectors(ctx, "m", store.Query{}, func(v store.VecScan) error {
		var rec store.Record
		for _, r := range recs {
			if r.Key == v.Key {
				rec = r
			}
		}
		want, _ := truth.Embed(ctx, []string{store.DocText("Document: ", rec)})
		got := store.DecodeVec(v.Blob, nil)
		for i := range got {
			if math.Abs(float64(got[i]-want[0][i])) > 1e-6 {
				t.Errorf("%s: component %d is %v, want %v", v.Key, i, got[i], want[0][i])
				break
			}
		}
		seen++
		return nil
	})
	if err != nil || seen != len(recs) {
		t.Errorf("%d vectors for %d records, %v", seen, len(recs), err)
	}
}

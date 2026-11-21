package eval

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

func naiveCos(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func randomUnit(r *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	for {
		for i := range v {
			v[i] = float32(r.NormFloat64())
		}
		if vec.Normalize(v) == nil {
			return v
		}
	}
}

func TestVectorListsMatchNaiveReference(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	vocab := make([]string, 30)
	for i := range vocab {
		vocab[i] = fmt.Sprintf("word%c%c%c", 'a'+i%26, 'a'+(i*7)%26, 'a'+(i*3)%26)
	}
	sentence := func(n int) string {
		w := make([]string, n)
		for i := range w {
			w[i] = vocab[r.IntN(len(vocab))]
		}
		return strings.Join(w, " ")
	}
	ctx := t.Context()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	const nDocs = 90
	kinds := []store.Kind{store.Fact, store.Rule, store.Log, store.Fact}
	for i := range nDocs {
		key := fmt.Sprintf("rec_%03d", i)
		title, body := sentence(3), sentence(20)
		kind := kinds[i%len(kinds)]
		if _, err := s.Put(ctx, store.PutRequest{Key: key, Kind: kind, Scopes: []string{store.GlobalScope}, Title: &title, Body: &body,
			Gate: store.Gate{Dup: 2, Rel: 2, MinShared: 1000}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"rec_005", "rec_010"} {
		if _, err := s.Retire(ctx, k, "test"); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := s.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	const dim = 12
	docs := &vec.Set{Dim: dim}
	byKey := map[string]store.Record{}
	for i, rc := range recs {
		docs.Names = append(docs.Names, rc.Key)
		byKey[rc.Key] = rc
		var row []float32
		if i%9 == 4 && i > 0 {
			row = slices.Clone(docs.Row(i - 1))
		} else {
			row = randomUnit(r, dim)
		}
		docs.Data = append(docs.Data, row...)
	}
	queries := &vec.Set{Dim: dim}
	var qs []*query
	for i := range 40 {
		q := &query{Prompt: Prompt{Session: "s", Turn: i + 1, Text: sentence(6), Query: sentence(2 + i%6)}}
		qs = append(qs, q)
		queries.Names = append(queries.Names, queryID(q))
		queries.Data = append(queries.Data, randomUnit(r, dim)...)
	}
	for _, c := range []struct {
		w     float64
		depth int
	}{{1, 20}, {0.5, 60}, {2, 12}, {0, 60}} {
		ev := &vecEval{s: s, cfg: Config{VecWeight: c.w, VecDepth: c.depth}, docs: docs, queries: queries, qIndex: map[string]int{}, index: map[string]int{}, keySlug: map[string]string{}, isLog: make([]bool, len(docs.Names)), pool: 12, settings: recall.DefaultSettings()}
		for i, n := range queries.Names {
			ev.qIndex[n] = i
		}
		for i, n := range docs.Names {
			ev.index[n] = i
			ev.isLog[i] = byKey[n].Kind == store.Log
			if byKey[n].Status == store.Current {
				ev.eligAll = append(ev.eligAll, i)
				if byKey[n].Kind != store.Log {
					ev.elig = append(ev.elig, i)
				}
			}
		}
		for _, q := range qs {
			got, err := ev.one(ctx, q, true)
			if err != nil {
				t.Fatal(err)
			}
			qv := queries.Row(ev.qIndex[queryID(q)])
			type cs struct {
				key string
				cos float64
			}
			var all []cs
			for i, n := range docs.Names {
				if byKey[n].Status == store.Current && byKey[n].Kind != store.Log {
					all = append(all, cs{n, naiveCos(qv, docs.Row(i))})
				}
			}
			sort.SliceStable(all, func(i, j int) bool {
				if all[i].cos != all[j].cos {
					return all[i].cos > all[j].cos
				}
				return all[i].key < all[j].key
			})
			var wantVec []string
			for _, x := range all[:12] {
				wantVec = append(wantVec, x.key)
			}
			var gotVec []string
			for _, h := range got.vec {
				gotVec = append(gotVec, h.key)
			}
			if !slices.Equal(gotVec, wantVec) {
				t.Fatalf("w %v depth %d %s vector list %v want %v", c.w, c.depth, queryID(q), gotVec, wantVec)
			}
			for i, h := range got.vec {
				if h.cos != all[i].cos {
					t.Fatalf("cos %v want %v", h.cos, all[i].cos)
				}
			}
			vrank := map[string]int{}
			for i, x := range all[:min(c.depth, len(all))] {
				vrank[x.key] = i + 1
			}
			hits, err := s.Search(ctx, store.Query{Text: q.Query, Limit: 1000, Depth: max(recall.SearchDepth, 12), ExcludeKinds: []store.Kind{store.Log}, Mode: store.Fused})
			if err != nil {
				t.Fatal(err)
			}
			type fs struct {
				key   string
				score float64
				bm    float64
			}
			scores := map[string]*fs{}
			for _, h := range hits {
				scores[h.Record.Key] = &fs{h.Record.Key, h.Score, h.WordsBM25 + h.TriBM25}
			}
			if c.w > 0 {
				for k, rk := range vrank {
					f := scores[k]
					if f == nil {
						f = &fs{key: k}
						scores[k] = f
					}
					f.score += c.w / float64(store.RRFK+rk)
				}
			}
			var ref []*fs
			for _, f := range scores {
				ref = append(ref, f)
			}
			sort.Slice(ref, func(i, j int) bool {
				a, b := ref[i], ref[j]
				if a.score != b.score {
					return a.score > b.score
				}
				if a.bm != b.bm {
					return a.bm > b.bm
				}
				return a.key < b.key
			})
			var wantFus, gotFus []string
			for _, f := range ref[:min(12, len(ref))] {
				wantFus = append(wantFus, f.key)
			}
			for _, h := range got.fus {
				gotFus = append(gotFus, h.key)
			}
			if !slices.Equal(gotFus, wantFus) {
				t.Fatalf("w %v depth %d %s fused list %v want %v", c.w, c.depth, queryID(q), gotFus, wantFus)
			}
			if got.mismatch {
				t.Fatalf("lexical reference mismatch")
			}
			var wantAll []string
			var allDocs []cs
			for i, n := range docs.Names {
				if byKey[n].Status == store.Current {
					allDocs = append(allDocs, cs{n, naiveCos(qv, docs.Row(i))})
				}
			}
			sort.SliceStable(allDocs, func(i, j int) bool {
				if allDocs[i].cos != allDocs[j].cos {
					return allDocs[i].cos > allDocs[j].cos
				}
				return allDocs[i].key < allDocs[j].key
			})
			for _, x := range allDocs[:12] {
				wantAll = append(wantAll, x.key)
			}
			var gotAll []string
			for _, h := range got.all {
				gotAll = append(gotAll, h.key)
			}
			if !slices.Equal(gotAll, wantAll) {
				t.Fatalf("vector-all %v want %v", gotAll, wantAll)
			}
		}
	}
}

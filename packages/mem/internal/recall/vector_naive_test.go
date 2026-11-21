package recall

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"

	"mem/internal/store"
)

type naiveDoc struct {
	key, file string
	log       bool
	vec       []float32
}

type naiveHit struct {
	key, file string
	log       bool
	cos       float64
}

func naiveCos(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func naiveFirstPerFile(in []naiveHit, n int) []naiveHit {
	var out []naiveHit
	for _, h := range in {
		dup := false
		for _, o := range out {
			if o.file == h.file {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		out = append(out, h)
		if len(out) >= n {
			break
		}
	}
	return out
}

func naiveGate(in []naiveHit, g KindGate) []string {
	if g.Take <= 0 || len(in) == 0 || in[0].cos < g.MinCos {
		return nil
	}
	if g.MinMargin > NoMargin {
		second := 0.0
		if len(in) > 1 {
			second = in[1].cos
		}
		if in[0].cos-second < g.MinMargin {
			return nil
		}
	}
	var out []string
	for pos, h := range in {
		if pos < g.Take && h.cos >= g.MinCos {
			out = append(out, h.key)
		}
	}
	return out
}

func naivePool(docs []naiveDoc, q []float32, v VectorSettings, words, tri []store.ListHit) (reg, logs []naiveHit) {
	all := make([]naiveHit, len(docs))
	cosOf := map[string]float64{}
	docOf := map[string]naiveDoc{}
	for i, d := range docs {
		all[i] = naiveHit{d.key, d.file, d.log, naiveCos(q, d.vec)}
		cosOf[d.key] = all[i].cos
		docOf[d.key] = d
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].cos != all[j].cos {
			return all[i].cos > all[j].cos
		}
		return all[i].key < all[j].key
	})
	var regOnly []naiveHit
	for _, h := range all {
		if !h.log {
			regOnly = append(regOnly, h)
		}
	}
	var pool []naiveHit
	switch v.Pool {
	case PoolReg:
		pool = naiveFirstPerFile(regOnly, v.PoolSize)
	case PoolAll:
		pool = naiveFirstPerFile(all, v.PoolSize)
	default:
		var vk []naiveHit
		if v.Pool == PoolFusedReg {
			vk = regOnly[:min(v.Depth, len(regOnly))]
		} else {
			vk = naiveFirstPerFile(all, v.Depth)
		}
		type row struct {
			key     string
			score   float64
			bm      float64
			w, t, r int
		}
		rows := map[string]*row{}
		at := func(k string) *row {
			if rows[k] == nil {
				rows[k] = &row{key: k}
			}
			return rows[k]
		}
		for _, h := range words {
			r := at(h.Key)
			r.w = h.Rank
			r.bm += h.Score
		}
		for _, h := range tri {
			r := at(h.Key)
			r.t = h.Rank
			r.bm += h.Score
		}
		if v.Weights.Vec > 0 {
			for i, h := range vk {
				at(h.key).r = i + 1
			}
		}
		var list []*row
		for _, r := range rows {
			if r.w > 0 {
				r.score += v.Weights.Words / float64(60+r.w)
			}
			if r.t > 0 {
				r.score += v.Weights.Tri / float64(60+r.t)
			}
			if r.r > 0 {
				r.score += v.Weights.Vec / float64(60+r.r)
			}
			list = append(list, r)
		}
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if a.score != b.score {
				return a.score > b.score
			}
			if a.bm != b.bm {
				return a.bm > b.bm
			}
			return a.key < b.key
		})
		if v.Pool == PoolFusedReg && len(list) > v.PoolSize {
			list = list[:v.PoolSize]
		}
		var fused []naiveHit
		for _, r := range list {
			d, ok := docOf[r.key]
			h := naiveHit{key: r.key, file: r.key, cos: cosOf[r.key]}
			if ok {
				h.file, h.log = d.file, d.log
			}
			fused = append(fused, h)
		}
		pool = naiveFirstPerFile(fused, v.PoolSize)
	}
	for _, h := range pool {
		if h.log {
			logs = append(logs, h)
		} else {
			reg = append(reg, h)
		}
	}
	return reg, logs
}

func randomUnit(r *rand.Rand, dim int, quant bool) []float32 {
	v := make([]float32, dim)
	var s float64
	for i := range v {
		x := r.NormFloat64()
		if quant {
			x = math.Round(x)
		}
		v[i] = float32(x)
		s += float64(v[i]) * float64(v[i])
	}
	if s == 0 {
		v[0], s = 1, 1
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / math.Sqrt(s))
	}
	return v
}

func TestVectorPipelineAgainstNaiveReference(t *testing.T) {
	const dim = 5
	r := rand.New(rand.NewSource(7))
	pools := []Pool{PoolReg, PoolFusedReg, PoolAll, PoolFusedAll}
	for iter := 0; iter < 3000; iter++ {
		n := 1 + r.Intn(40)
		quant := iter%3 == 0
		var docs []naiveDoc
		var entries []Entry
		for i := 0; i < n; i++ {
			d := naiveDoc{key: fmt.Sprintf("k%02d", i), vec: randomUnit(r, dim, quant)}
			d.file = d.key
			if r.Intn(3) == 0 {
				d.log = true
				d.file = fmt.Sprintf("f%d", r.Intn(4))
				d.key = fmt.Sprintf("%s#%d", d.file, i)
			}
			if r.Intn(8) == 0 && len(docs) > 0 {
				d.vec = slices.Clone(docs[r.Intn(len(docs))].vec)
			}
			docs = append(docs, d)
			entries = append(entries, Entry{Key: d.key, File: d.file, Log: d.log, Vec: d.vec})
		}
		keys := make([]string, 0, n+3)
		for _, d := range docs {
			if !d.log {
				keys = append(keys, d.key)
			}
		}
		keys = append(keys, "nov1", "nov2")
		list := func() []store.ListHit {
			perm := r.Perm(len(keys))
			m := r.Intn(len(keys) + 1)
			var out []store.ListHit
			for rank := 1; rank <= m; rank++ {
				out = append(out, store.ListHit{Key: keys[perm[rank-1]], Rank: rank, Score: math.Round(r.Float64()*100) / 10})
			}
			return out
		}
		words, tri := list(), list()
		v := VectorSettings{
			Pool:     pools[r.Intn(len(pools))],
			Depth:    1 + r.Intn(30),
			PoolSize: 1 + r.Intn(15),
			Weights:  Weights{Words: float64(r.Intn(3)), Tri: float64(r.Intn(3)) * 0.3, Vec: float64(r.Intn(3))},
			Reg:      KindGate{Take: r.Intn(6), MinCos: r.Float64()*1.4 - 0.4, MinMargin: []float64{NoMargin, 0, 0.05, 0.2}[r.Intn(4)]},
			Log:      KindGate{Take: r.Intn(4), MinCos: r.Float64()*1.4 - 0.4, MinMargin: []float64{NoMargin, 0, 0.05}[r.Intn(3)]},
		}
		if !v.Pool.Fused() {
			words, tri = nil, nil
		}
		q := randomUnit(r, dim, quant)
		ix := NewIndex(entries)
		scored, err := ix.Score(q)
		if err != nil {
			t.Fatal(err)
		}
		sel := v.Select(ix, scored, words, tri)
		wantReg, wantLog := naivePool(docs, q, v, words, tri)
		got := map[string][]string{"reg": keysOf(sel.Reg), "log": keysOf(sel.Log)}
		want := map[string][]string{"reg": naiveGate(wantReg, v.Reg), "log": naiveGate(wantLog, v.Log)}
		for _, kind := range []string{"reg", "log"} {
			if !slices.Equal(got[kind], want[kind]) && len(got[kind])+len(want[kind]) > 0 {
				t.Fatalf("iter %d pool %s %s: %v, want %v\nsettings %+v", iter, v.Pool, kind, got[kind], want[kind], v)
			}
		}
		for _, h := range append(append([]VHit{}, sel.Reg...), sel.Log...) {
			for _, d := range docs {
				if d.key == h.Key && math.Abs(h.Cos-naiveCos(q, d.vec)) > 1e-12 {
					t.Fatalf("iter %d: cosine of %s is %v, want %v", iter, h.Key, h.Cos, naiveCos(q, d.vec))
				}
			}
		}
		files := map[string]bool{}
		for _, h := range append(append([]VHit{}, sel.Reg...), sel.Log...) {
			if files[h.File] {
				t.Fatalf("iter %d: file %s twice", iter, h.File)
			}
			files[h.File] = true
		}
	}
}

package recall

import (
	"cmp"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"

	"mem/internal/store"
	"mem/internal/text"
)

const (
	DefaultPairsMinCos = 0.9
	DefaultPairsLimit  = 50
)

type PairOptions struct {
	MinCos float64
	All    bool
	Limit  int
}

type Pair struct {
	A      string     `json:"a"`
	B      string     `json:"b"`
	KindA  store.Kind `json:"kind_a"`
	KindB  store.Kind `json:"kind_b"`
	Cos    float64    `json:"cos"`
	Sim    float64    `json:"sim"`
	Shared int        `json:"shared"`
	Gate   bool       `json:"gate"`
}

type PairResult struct {
	MinCos  float64 `json:"min_cos"`
	Model   string  `json:"model"`
	Records int     `json:"records"`
	Total   int     `json:"total"`
	Gated   int     `json:"gated"`
	Past    int     `json:"past"`
	Pairs   []Pair  `json:"pairs"`
}

type pairHit struct {
	i, j int
	cos  float64
}

func sharesScope(a, b store.Record) bool {
	if a.HasScope(store.GlobalScope) || b.HasScope(store.GlobalScope) {
		return true
	}
	return slices.ContainsFunc(a.Scopes, b.HasScope)
}

func round3(x float64) float64 {
	return math.Round(x*1000) / 1000
}

func Pairs(recs []store.Record, vecs map[string][]float32, o PairOptions) PairResult {
	res := PairResult{MinCos: o.MinCos, Pairs: []Pair{}}
	var rs []store.Record
	for _, r := range recs {
		if v, ok := vecs[r.Key]; ok && len(v) > 0 && r.Status == store.Current && r.Kind != store.Log {
			rs = append(rs, r)
		}
	}
	slices.SortFunc(rs, func(a, b store.Record) int { return strings.Compare(a.Key, b.Key) })
	vs := make([][]float32, len(rs))
	for i, r := range rs {
		vs[i] = normTopicVec(vecs[r.Key])
	}
	res.Records = len(rs)
	workers := max(1, min(runtime.GOMAXPROCS(0), len(rs)))
	rows := make(chan int)
	found := make([][]pairHit, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range rows {
				for j := i + 1; j < len(vs); j++ {
					if len(vs[i]) != len(vs[j]) {
						continue
					}
					if c := dot(vs[i], vs[j]); c >= o.MinCos {
						found[w] = append(found[w], pairHit{i, j, c})
					}
				}
			}
		})
	}
	for i := range rs {
		rows <- i
	}
	close(rows)
	wg.Wait()
	sets := map[int]text.Set{}
	set := func(i int) text.Set {
		s, ok := sets[i]
		if !ok {
			s = text.NewSet(store.RecordText(rs[i]))
			sets[i] = s
		}
		return s
	}
	gate := store.DefaultGate()
	var all []pairHit
	for _, f := range found {
		all = append(all, f...)
	}
	slices.SortFunc(all, func(x, y pairHit) int {
		if c := cmp.Compare(y.cos, x.cos); c != 0 {
			return c
		}
		if c := strings.Compare(rs[x.i].Key, rs[y.i].Key); c != 0 {
			return c
		}
		return strings.Compare(rs[x.j].Key, rs[y.j].Key)
	})
	for _, h := range all {
		a, b := rs[h.i], rs[h.j]
		sa, sb := set(h.i), set(h.j)
		c := store.Candidate{Similarity: sa.Similarity(sb), Shared: sa.Intersect(sb)}
		p := Pair{A: a.Key, B: b.Key, KindA: a.Kind, KindB: b.Kind, Cos: round3(h.cos), Sim: round3(c.Similarity), Shared: c.Shared,
			Gate: a.Kind == b.Kind && gate.Duplicate(c) && sharesScope(a, b)}
		res.Total++
		if p.Gate {
			res.Gated++
		} else {
			res.Past++
		}
		if (o.All || !p.Gate) && (o.Limit <= 0 || len(res.Pairs) < o.Limit) {
			res.Pairs = append(res.Pairs, p)
		}
	}
	return res
}

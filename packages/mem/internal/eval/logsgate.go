package eval

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sync"

	"mem/internal/recall"
)

type KindGate struct {
	Take      int     `json:"take"`
	MinCos    float64 `json:"min_cos"`
	MinMargin float64 `json:"min_margin"`
}

func (g KindGate) apply(list []vh) []vh {
	if g.Take <= 0 || len(list) == 0 || list[0].cos < g.MinCos {
		return nil
	}
	if g.MinMargin > noMargin {
		second := 0.0
		if len(list) > 1 {
			second = list[1].cos
		}
		if list[0].cos-second < g.MinMargin {
			return nil
		}
	}
	var out []vh
	for i := 0; i < len(list) && i < g.Take; i++ {
		if list[i].cos >= g.MinCos {
			out = append(out, list[i])
		}
	}
	return out
}

func fileID(h vh) string {
	if h.slug == "" {
		return h.key
	}
	return h.slug
}

func dedupeFiles(list []vh, n int) []vh {
	seen := map[string]bool{}
	var out []vh
	for _, h := range list {
		id := fileID(h)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, h)
		if len(out) == n {
			break
		}
	}
	return out
}

type kindCap struct {
	n    int
	seen map[string]bool
	per  [2]int
	out  []vh
}

func newKindCap(n int) *kindCap { return &kindCap{n: n, seen: map[string]bool{}} }

func (c *kindCap) add(h vh) bool {
	kind := 0
	if h.log {
		kind = 1
	}
	id := fileID(h)
	if !c.seen[id] && c.per[kind] < c.n {
		c.seen[id] = true
		c.per[kind]++
		c.out = append(c.out, h)
	}
	return c.per[0] >= c.n && c.per[1] >= c.n
}

func splitKinds(list []vh) ([]vh, []vh) {
	var reg, logs []vh
	for _, h := range list {
		if h.log {
			logs = append(logs, h)
		} else {
			reg = append(reg, h)
		}
	}
	return reg, logs
}

func foldK(session string, k int) int {
	if k <= 1 {
		return 0
	}
	sum := sha256.Sum256([]byte(session))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(k))
}

type promptInput struct {
	fold    int
	goldLen int
	gold    []string
	reg     []vh
	logs    []vh
	lex     []vh
	lexSets [][]vh
	lexGate []uint16
}

type foldTotals struct {
	rec      float64
	hit, neg int
	goldN    int
	nN       int
}

type settingStats struct {
	rec      []float64
	hit, neg [][]uint64
}

type layout struct {
	k               int
	gold, neg       []promptInput
	goldN, negN     []int
	goldBit, negBit []int
}

func newLayout(k int, gold, neg []promptInput) *layout {
	l := &layout{k: k, gold: gold, neg: neg, goldN: make([]int, k), negN: make([]int, k)}
	for _, p := range gold {
		l.goldBit = append(l.goldBit, l.goldN[p.fold])
		l.goldN[p.fold]++
	}
	for _, p := range neg {
		l.negBit = append(l.negBit, l.negN[p.fold])
		l.negN[p.fold]++
	}
	return l
}

func words(n int) int { return (n + 63) / 64 }

func (l *layout) stats(items func(*promptInput) []string) settingStats {
	st := settingStats{rec: make([]float64, l.k), hit: make([][]uint64, l.k), neg: make([][]uint64, l.k)}
	for f := range l.k {
		st.hit[f] = make([]uint64, words(l.goldN[f]))
		st.neg[f] = make([]uint64, words(l.negN[f]))
	}
	for i := range l.gold {
		p := &l.gold[i]
		its := items(p)
		n := 0
		for j, s := range its {
			if slices.Contains(p.gold, s) && !slices.Contains(its[:j], s) {
				n++
			}
		}
		if n == 0 {
			continue
		}
		st.rec[p.fold] += float64(n) / float64(p.goldLen)
		b := l.goldBit[i]
		st.hit[p.fold][b/64] |= 1 << (b % 64)
	}
	for i := range l.neg {
		p := &l.neg[i]
		if len(items(p)) == 0 {
			continue
		}
		b := l.negBit[i]
		st.neg[p.fold][b/64] |= 1 << (b % 64)
	}
	return st
}

func orCount(a, b []uint64) int {
	n := 0
	for i := range a {
		n += bits.OnesCount64(a[i] | b[i])
	}
	return n
}

func (l *layout) combineInto(a, b *settingStats, out []foldTotals) {
	for f := range l.k {
		out[f] = foldTotals{rec: a.rec[f] + b.rec[f], hit: orCount(a.hit[f], b.hit[f]), neg: orCount(a.neg[f], b.neg[f]), goldN: l.goldN[f], nN: l.negN[f]}
	}
}

func (l *layout) combine(a, b *settingStats) []foldTotals {
	out := make([]foldTotals, l.k)
	l.combineInto(a, b, out)
	return out
}

type pickResult struct {
	Reg, Log int
	Feasible bool
	Train    foldTotals
}

func trainSum(tot []foldTotals, held int) foldTotals {
	var s foldTotals
	for f, t := range tot {
		if f == held {
			continue
		}
		s.rec += t.rec
		s.hit += t.hit
		s.neg += t.neg
		s.goldN += t.goldN
		s.nN += t.nN
	}
	return s
}

func quantized(x float64) int64 { return int64(math.Round(x * 1e9)) }

type candidate struct {
	idx  int
	t    foldTotals
	live bool
}

func betterFeasible(a, b candidate) bool {
	if !b.live {
		return true
	}
	ra, rb := quantized(a.t.rec), quantized(b.t.rec)
	switch {
	case ra != rb:
		return ra > rb
	case a.t.hit != b.t.hit:
		return a.t.hit > b.t.hit
	case a.t.neg != b.t.neg:
		return a.t.neg < b.t.neg
	}
	return a.idx < b.idx
}

func betterFallback(a, b candidate) bool {
	if !b.live {
		return true
	}
	ra, rb := quantized(a.t.rec), quantized(b.t.rec)
	switch {
	case a.t.neg != b.t.neg:
		return a.t.neg < b.t.neg
	case ra != rb:
		return ra > rb
	case a.t.hit != b.t.hit:
		return a.t.hit > b.t.hit
	}
	return a.idx < b.idx
}

type bestSlot struct{ feasible, fallback candidate }

func (l *layout) crossFit(reg, logs []settingStats, budgets []float64, nw int) [][]pickResult {
	nw = max(1, min(nw, len(reg)))
	merged := make([][]bestSlot, len(budgets))
	for b := range merged {
		merged[b] = make([]bestSlot, l.k)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	next := make(chan int)
	for range nw {
		wg.Go(func() {
			local := make([][]bestSlot, len(budgets))
			for b := range local {
				local[b] = make([]bestSlot, l.k)
			}
			tot := make([]foldTotals, l.k)
			for i := range next {
				for j := range logs {
					l.combineInto(&reg[i], &logs[j], tot)
					idx := i*len(logs) + j
					for f := range l.k {
						t := trainSum(tot, f)
						c := candidate{idx: idx, t: t, live: true}
						for b, budget := range budgets {
							s := &local[b][f]
							if float64(t.neg) <= budget*float64(t.nN)+1e-9 {
								if betterFeasible(c, s.feasible) {
									s.feasible = c
								}
							} else if betterFallback(c, s.fallback) {
								s.fallback = c
							}
						}
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for b := range local {
				for f := range local[b] {
					m, s := &merged[b][f], local[b][f]
					if s.feasible.live && betterFeasible(s.feasible, m.feasible) {
						m.feasible = s.feasible
					}
					if s.fallback.live && betterFallback(s.fallback, m.fallback) {
						m.fallback = s.fallback
					}
				}
			}
		})
	}
	for i := range reg {
		next <- i
	}
	close(next)
	wg.Wait()
	out := make([][]pickResult, len(budgets))
	for b := range out {
		out[b] = make([]pickResult, l.k)
		for f := range out[b] {
			s := merged[b][f]
			c, ok := s.feasible, true
			if !c.live {
				c, ok = s.fallback, false
			}
			out[b][f] = pickResult{Reg: c.idx / len(logs), Log: c.idx % len(logs), Feasible: ok, Train: c.t}
		}
	}
	return out
}

type ChunkStats struct {
	GoldLogPrompts   int     `json:"gold_prompts_with_log_gold"`
	GoldInPool       int     `json:"gold_log_file_in_raw_top12"`
	GoldMulti2       int     `json:"gold_file_two_or_more_chunks_in_top12"`
	GoldMulti3       int     `json:"gold_file_three_or_more_chunks_in_top12"`
	GoldMeanChunks   float64 `json:"gold_file_mean_chunks_in_top12"`
	TopLogGold       int     `json:"gold_prompts_top1_is_log_chunk"`
	TopLogRepeatGold int     `json:"gold_prompts_top1_file_repeats_in_top12"`
	TopLogNeg        int     `json:"negative_prompts_top1_is_log_chunk"`
	TopLogRepeatNeg  int     `json:"negative_prompts_top1_file_repeats_in_top12"`
}

type LGFold struct {
	Fold        int          `json:"fold"`
	Regular     KindGate     `json:"regular_gate"`
	Lexical     bool         `json:"regular_is_shipped_lexical_gate"`
	LexGate     *recall.Gate `json:"lexical_gate,omitempty"`
	Log         KindGate     `json:"log_gate"`
	Feasible    bool         `json:"feasible_on_train"`
	TrainRecall float64      `json:"train_recall"`
	TrainFIR    float64      `json:"train_fir"`
	Gold        int          `json:"gold_prompts"`
	Negatives   int          `json:"negative_prompts"`
	Recall      float64      `json:"held_out_recall"`
	FIR         float64      `json:"held_out_fir"`
}

type LGResult struct {
	Variant    string   `json:"variant"`
	Pool       string   `json:"pool"`
	Budget     float64  `json:"fir_budget"`
	Gold       int      `json:"gold_prompts"`
	Negatives  int      `json:"negative_prompts"`
	Recall     float64  `json:"recall"`
	Hit        float64  `json:"hit"`
	FIR        float64  `json:"fir"`
	GoldChars  float64  `json:"mean_chars_gold"`
	NegChars   float64  `json:"mean_chars_negative"`
	Records    int      `json:"records_injected"`
	LogRecords int      `json:"log_records_injected"`
	LogShare   float64  `json:"log_share_of_injections"`
	LogPrompts int      `json:"prompts_with_log_injection"`
	Dropped    int      `json:"records_dropped_by_pack"`
	Infeasible int      `json:"folds_without_feasible_pick"`
	Spread     []string `json:"parameter_spread"`
	Folds      []LGFold `json:"folds"`
}

type LogsGateSummary struct {
	K       int        `json:"folds"`
	Budgets []float64  `json:"fir_budgets"`
	Pools   []string   `json:"pools"`
	RegGrid int        `json:"regular_grid"`
	LogGrid int        `json:"log_grid"`
	Chunks  ChunkStats `json:"chunk_stats"`
	Lexical LGResult   `json:"lexical_reference"`
	Results []LGResult `json:"results"`
}

const lexSweepVariant = "lex-sweep"

var lexGrid = sync.OnceValue(SweepGrid)

func sweepPicks(st recall.Settings, cands []recall.Candidate) ([][]string, []uint16) {
	grid := lexGrid()
	gate := make([]uint16, len(grid))
	var sets [][]string
	seen := map[string]uint16{}
	for gi, g := range grid {
		idx := g.Choose(cands, st.Limit)
		k := fmt.Sprint(idx)
		id, ok := seen[k]
		if !ok {
			id = uint16(len(sets))
			seen[k] = id
			keys := make([]string, len(idx))
			for i, j := range idx {
				keys[i] = cands[j].Hit.Record.Key
			}
			sets = append(sets, keys)
		}
		gate[gi] = id
	}
	return sets, gate
}

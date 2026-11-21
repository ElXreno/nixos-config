package eval

import (
	"bufio"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"

	"mem/internal/recall"
)

type SweepRow struct {
	Gate       recall.Gate `json:"gate"`
	Gold       HookStats   `json:"gold"`
	Negative   HookStats   `json:"negative"`
	Folds      [2]Fold     `json:"folds"`
	Pareto     bool        `json:"pareto"`
	Knee       bool        `json:"knee"`
	FoldPareto [2]bool     `json:"fold_pareto"`
	FoldKnee   [2]bool     `json:"fold_knee"`
}

func SweepGrid() []recall.Gate {
	var out []recall.Gate
	for c := 0; c <= 12; c++ {
		for shared := 0; shared <= 6; shared++ {
			for topic := 0; topic <= 2; topic++ {
				g := recall.Gate{MinCover: math.Round(float64(c)*5) / 100, MinShared: shared, MinTopic: topic}
				out = append(out, g)
				for ls := 1; ls <= 4; ls++ {
					g.Lead, g.LeadShared = true, ls
					out = append(out, g)
				}
			}
		}
	}
	return out
}

type CrossFit struct {
	PickedOn  int         `json:"picked_on_fold"`
	Gate      recall.Gate `json:"gate"`
	InFold    Fold        `json:"in_fold"`
	HeldOut   Fold        `json:"held_out"`
	HeldKnee  recall.Gate `json:"held_out_knee"`
	HeldBest  Fold        `json:"held_out_knee_stats"`
	HeldAtFIR float64     `json:"held_out_best_recall_at_same_fir"`
}

func crossFit(rows []SweepRow) []CrossFit {
	knee := func(f int) int {
		for i, r := range rows {
			if r.FoldKnee[f] {
				return i
			}
		}
		return -1
	}
	var out []CrossFit
	for f := range 2 {
		a, b := knee(f), knee(1-f)
		if a < 0 || b < 0 {
			continue
		}
		held := rows[a].Folds[1-f]
		best := 0.0
		for _, r := range rows {
			if r.Folds[1-f].FalseRate <= held.FalseRate {
				best = max(best, r.Folds[1-f].Recall)
			}
		}
		out = append(out, CrossFit{PickedOn: f, Gate: rows[a].Gate, InFold: rows[a].Folds[f], HeldOut: held,
			HeldKnee: rows[b].Gate, HeldBest: rows[b].Folds[1-f], HeldAtFIR: best})
	}
	return out
}

type sweepKey struct {
	q   *query
	sig string
}

func runSetting(st recall.Settings, qs []*query, cache map[sweepKey]outcome) []outcome {
	out := make([]outcome, len(qs))
	for i, q := range qs {
		idx := st.Choose(q.cands, st.Limit)
		k := sweepKey{q, fmt.Sprint(idx)}
		o, ok := cache[k]
		if !ok {
			o = outcomeOf(st.Inject(q.Query, q.cands))
			cache[k] = o
		}
		out[i] = o
	}
	return out
}

func sweep(gold, neg []*query, base recall.Settings, keySlug map[string]string, grid []recall.Gate, nw int) []SweepRow {
	rows := make([]SweepRow, len(grid))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range nw {
		wg.Go(func() {
			cache := map[sweepKey]outcome{}
			for gi := range jobs {
				st := base
				st.Gate = grid[gi]
				gout, nout := runSetting(st, gold, cache), runSetting(st, neg, cache)
				rows[gi] = SweepRow{
					Gate:     grid[gi],
					Gold:     hookStats(SetGold, gold, gout, keySlug),
					Negative: hookStats(SetNegative, neg, nout, keySlug),
					Folds:    folds(gold, gout, neg, nout, keySlug),
				}
			}
		})
	}
	for gi := range grid {
		jobs <- gi
	}
	close(jobs)
	wg.Wait()
	markFrontier(rows)
	return rows
}

type point struct{ r, f float64 }

func frontier(pts []point) ([]bool, int) {
	pareto := make([]bool, len(pts))
	var front []int
	for i, p := range pts {
		pareto[i] = !slices.ContainsFunc(pts, func(o point) bool { return o.r >= p.r && o.f <= p.f && (o.r > p.r || o.f < p.f) })
		if pareto[i] {
			front = append(front, i)
		}
	}
	maxR, maxF := 0.0, 0.0
	for _, i := range front {
		maxR = max(maxR, pts[i].r)
		maxF = max(maxF, pts[i].f)
	}
	if maxR == 0 || maxF == 0 {
		return pareto, -1
	}
	best, bestV := -1, math.Inf(-1)
	sort.SliceStable(front, func(a, b int) bool { return pts[front[a]].f < pts[front[b]].f })
	for _, i := range front {
		if v := pts[i].r/maxR - pts[i].f/maxF; v > bestV {
			best, bestV = i, v
		}
	}
	return pareto, best
}

func markFrontier(rows []SweepRow) {
	pts := make([]point, len(rows))
	for i, r := range rows {
		pts[i] = point{r.Gold.Recall, r.Negative.InjectRate}
	}
	pareto, knee := frontier(pts)
	for i := range rows {
		rows[i].Pareto = pareto[i]
	}
	if knee >= 0 {
		rows[knee].Knee = true
	}
	for f := range 2 {
		for i, r := range rows {
			pts[i] = point{r.Folds[f].Recall, r.Folds[f].FalseRate}
		}
		pareto, knee := frontier(pts)
		for i := range rows {
			rows[i].FoldPareto[f] = pareto[i]
		}
		if knee >= 0 {
			rows[knee].FoldKnee[f] = true
		}
	}
}

func writeSweep(path string, rows []SweepRow) error {
	return writeFile(path, func(w *bufio.Writer) error {
		head := []string{"min_cover", "min_shared", "min_topic", "lead", "lead_shared", "gold_recall", "gold_hit", "gold_rate", "gold_precision", "gold_records",
			"gold_chars", "neg_fir", "neg_records", "neg_chars", "neg_chars_injecting", "recall_fold0", "fir_fold0", "recall_fold1", "fir_fold1", "pareto", "knee",
			"pareto_fold0", "knee_fold0", "pareto_fold1", "knee_fold1"}
		if _, err := fmt.Fprintln(w, strings.Join(head, "\t")); err != nil {
			return err
		}
		for _, r := range rows {
			g, n := r.Gold, r.Negative
			_, err := fmt.Fprintf(w, "%.2f\t%d\t%d\t%s\t%d\t%.4f\t%.4f\t%.4f\t%.4f\t%.3f\t%.1f\t%.4f\t%.3f\t%.1f\t%.1f\t%.4f\t%.4f\t%.4f\t%.4f\t%s\t%s\t%s\t%s\t%s\t%s\n",
				r.Gate.MinCover, r.Gate.MinShared, r.Gate.MinTopic, boolCol(r.Gate.Lead), r.Gate.LeadShared, g.Recall, g.Hit, g.InjectRate, g.Precision, g.MeanRecords,
				g.MeanChars, n.InjectRate, n.MeanRecords, n.MeanChars, n.MeanCharsInjecting, r.Folds[0].Recall, r.Folds[0].FalseRate,
				r.Folds[1].Recall, r.Folds[1].FalseRate, boolCol(r.Pareto), boolCol(r.Knee),
				boolCol(r.FoldPareto[0]), boolCol(r.FoldKnee[0]), boolCol(r.FoldPareto[1]), boolCol(r.FoldKnee[1]))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func writeHook(hookPath, candPath string, sets map[string][]*query, keySlug map[string]string) error {
	names := []string{SetGold, SetNegative}
	err := writeFile(hookPath, func(w *bufio.Writer) error {
		head := "set\tsession\tturn\thash\tprompt60\tquery_chars\tcandidates\tpicked\tinjected\tcut\tchars\tgold\tgold_injected"
		if _, err := fmt.Fprintln(w, head); err != nil {
			return err
		}
		for _, set := range names {
			for _, q := range sets[set] {
				o := outcomeOf(q.inj)
				var picked []string
				for _, p := range q.inj.Picks {
					picked = append(picked, p.Hit.Record.Key)
				}
				n := 0
				for _, k := range o.keys {
					if slices.Contains(q.gold, keySlug[k]) {
						n++
					}
				}
				_, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%d\t%d\t%s\t%s\t%s\t%d\t%s\t%d\n", set, q.Session, q.Turn, PromptHash(q.Text),
					tsvField(promptPrefix(q.Text)), len([]rune(q.Query)), len(q.cands), strings.Join(picked, ","), strings.Join(o.keys, ","),
					strings.Join(q.inj.Cut, ","), o.chars, strings.Join(q.gold, ","), n)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return writeFile(candPath, func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "set\thash\trank\tkey\tgold\tscore\twords_bm25\ttri_bm25\tcover\tshared\tterms\ttopic"); err != nil {
			return err
		}
		for _, set := range names {
			for _, q := range sets[set] {
				hash := PromptHash(q.Text)
				for i, c := range q.cands {
					h, rv := c.Hit, c.Relevance
					_, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%.6f\t%.3f\t%.3f\t%.4f\t%d\t%d\t%d\n", set, hash, i+1, h.Record.Key,
						boolCol(slices.Contains(q.gold, keySlug[h.Record.Key])), h.Score, h.WordsBM25, h.TriBM25, rv.Cover, rv.Shared, rv.Terms, rv.Topic)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func writeHookSummary(path string, hs []HookStats) error {
	return writeFile(path, func(w *bufio.Writer) error {
		head := "set\tprompts\tinjecting\tinjection_rate\trecords\tcut\tmean_records\tmean_chars\tmean_chars_injecting\tinjected_recall\tinjected_hit\tgold_injected\tinjected_precision"
		if _, err := fmt.Fprintln(w, head); err != nil {
			return err
		}
		for _, h := range hs {
			_, err := fmt.Fprintf(w, "%s\t%d\t%d\t%.4f\t%d\t%d\t%.4f\t%.1f\t%.1f\t%.4f\t%.4f\t%d\t%.4f\n", h.Set, h.Prompts, h.Injecting, h.InjectRate,
				h.Records, h.Cut, h.MeanRecords, h.MeanChars, h.MeanCharsInjecting, h.Recall, h.Hit, h.GoldInjected, h.Precision)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

type Fold struct {
	Recall    float64 `json:"injected_recall"`
	FalseRate float64 `json:"false_injection_rate"`
	Gold      int     `json:"gold_prompts"`
	Negatives int     `json:"negative_prompts"`
}

func foldOf(session string) int {
	return int(PromptHash(session)[0]) & 1
}

func folds(gold []*query, gout []outcome, neg []*query, nout []outcome, keySlug map[string]string) [2]Fold {
	var out [2]Fold
	for f := range out {
		var gq, nq []*query
		var go2, no2 []outcome
		for i, q := range gold {
			if foldOf(q.Session) == f {
				gq, go2 = append(gq, q), append(go2, gout[i])
			}
		}
		for i, q := range neg {
			if foldOf(q.Session) == f {
				nq, no2 = append(nq, q), append(no2, nout[i])
			}
		}
		g, n := hookStats(SetGold, gq, go2, keySlug), hookStats(SetNegative, nq, no2, keySlug)
		out[f] = Fold{Recall: g.Recall, FalseRate: n.InjectRate, Gold: g.Prompts, Negatives: n.Prompts}
	}
	return out
}

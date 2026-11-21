package eval

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"mem/internal/recall"
	"mem/internal/store"
)

const (
	PoolReg      = "reg"
	PoolFusedReg = "fused-reg"
	PoolAll      = "all"
	PoolFusedAll = "fused-all"

	DefaultFolds   = 5
	DefaultBudgets = "0.1578,0.25"

	lexicalVariant = "lexical"
	unsetBudget    = "-"
)

var (
	AllPools    = []string{PoolReg, PoolFusedReg, PoolAll, PoolFusedAll}
	regTakes    = []int{1, 2, 4}
	logTakes    = []int{1, 2}
	gridCosFrom = 20
	gridCosTo   = 90
)

type lgVariant struct {
	name     string
	pool     string
	lexical  bool
	logs     bool
	lexSweep bool
}

func lgVariants(pools []string, lexRegular bool) []lgVariant {
	var out []lgVariant
	add := func(pool string) {
		if !slices.Contains(pools, pool) {
			return
		}
		switch pool {
		case PoolReg, PoolFusedReg:
			out = append(out, lgVariant{name: pool, pool: pool})
		default:
			out = append(out, lgVariant{name: pool, pool: pool, logs: true})
		}
	}
	for _, pool := range AllPools {
		add(pool)
	}
	if lexRegular {
		for _, pool := range []string{PoolAll, PoolFusedAll} {
			if slices.Contains(pools, pool) {
				out = append(out, lgVariant{name: "lex+" + pool, pool: pool, lexical: true, logs: true})
			}
		}
	}
	return out
}

func kindGrid(takes []int) []KindGate {
	out := []KindGate{{MinMargin: noMargin}}
	for _, take := range takes {
		for c := gridCosTo; c >= gridCosFrom; c-- {
			for m := len(marginValues) - 1; m >= 0; m-- {
				out = append(out, KindGate{Take: take, MinCos: float64(c) / 100, MinMargin: marginValues[m]})
			}
		}
	}
	return out
}

type regChoice struct {
	lexical bool
	gate    KindGate
	sweep   *int
	lexGate recall.Gate
}

func (c regChoice) inject(p *promptInput) []vh {
	if c.sweep != nil {
		return p.lexSets[p.lexGate[*c.sweep]]
	}
	if c.lexical {
		return p.lex
	}
	return c.gate.apply(p.reg)
}

func slugsOf(l []vh) []string {
	out := make([]string, len(l))
	for i, h := range l {
		out[i] = fileID(h)
	}
	return out
}

func parStats(l *layout, n int, items func(i int) func(*promptInput) []string, nw int) []settingStats {
	out := make([]settingStats, n)
	next := make(chan int)
	var wg sync.WaitGroup
	for range max(1, nw) {
		wg.Go(func() {
			for i := range next {
				out[i] = l.stats(items(i))
			}
		})
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

func chunkStats(gold []*query, raw, neg [][]vh, logSlug map[string]bool) ChunkStats {
	var st ChunkStats
	var chunkSum int
	for i, q := range gold {
		counts := map[string]int{}
		for _, h := range raw[i] {
			if h.log {
				counts[h.slug]++
			}
		}
		most, any := 0, false
		for _, g := range q.gold {
			if logSlug[g] {
				any = true
				most = max(most, counts[g])
			}
		}
		if any {
			st.GoldLogPrompts++
			if most > 0 {
				st.GoldInPool++
				chunkSum += most
				if most >= 2 {
					st.GoldMulti2++
				}
				if most >= 3 {
					st.GoldMulti3++
				}
			}
		}
		if len(raw[i]) > 0 && raw[i][0].log {
			st.TopLogGold++
			n := 0
			for _, h := range raw[i] {
				if h.log && h.slug == raw[i][0].slug {
					n++
				}
			}
			if n >= 2 {
				st.TopLogRepeatGold++
			}
		}
	}
	if st.GoldInPool > 0 {
		st.GoldMeanChunks = float64(chunkSum) / float64(st.GoldInPool)
	}
	for _, l := range neg {
		if len(l) == 0 || !l[0].log {
			continue
		}
		st.TopLogNeg++
		n := 0
		for _, h := range l {
			if h.log && h.slug == l[0].slug {
				n++
			}
		}
		if n >= 2 {
			st.TopLogRepeatNeg++
		}
	}
	return st
}

func (ev *vecEval) wantsPool(name string) bool {
	return slices.Contains(ev.cfg.Pools, name)
}

func (ev *vecEval) poolLists(res *qres, cos []float64, regOrdered, allOrdered []int, words, tri []store.ListHit) {
	res.pools = map[string][]vh{}
	n := ev.pool
	if ev.wantsPool(PoolReg) {
		res.pools[PoolReg] = ev.dedupe(regOrdered, cos, n)
	}
	if ev.wantsPool(PoolAll) {
		kc := newKindCap(n)
		for _, i := range allOrdered {
			if kc.add(ev.vh(i, cos)) {
				break
			}
		}
		res.pools[PoolAll] = kc.out
	}
	if ev.wantsPool(PoolFusedReg) {
		res.pools[PoolFusedReg] = dedupeFiles(res.fus, n)
	}
	if ev.wantsPool(PoolFusedAll) {
		var vkeys []string
		for _, h := range ev.dedupe(allOrdered, cos, ev.cfg.VecDepth) {
			vkeys = append(vkeys, h.key)
		}
		fused := fuse(words, tri, vkeys, ev.cfg.VecWeight, math.MaxInt)
		kc := newKindCap(n)
		for _, f := range fused {
			if kc.add(ev.vh(ev.index[f.key], cos)) {
				break
			}
		}
		res.pools[PoolFusedAll] = kc.out
	}
	for _, i := range allOrdered[:min(n, len(allOrdered))] {
		res.raw = append(res.raw, ev.vh(i, cos))
	}
}

type heldRow struct {
	q          *query
	set        string
	fold       int
	goldN      int
	injected   int
	injLog     int
	goldInj    int
	goldInjLog int
	dropped    int
	chars      int
}

func (ev *vecEval) decideRow(p *promptInput, q *query, set string, rc regChoice, lg KindGate, byKey map[string]store.Record) heldRow {
	row := heldRow{q: q, set: set, fold: p.fold, goldN: len(q.gold)}
	picked := append(slices.Clone(rc.inject(p)), lg.apply(p.logs)...)
	if len(picked) == 0 {
		return row
	}
	recs := make([]store.Record, len(picked))
	for i, h := range picked {
		recs[i] = byKey[h.key]
	}
	packed := recall.Pack(recall.PromptHeader, recs, q.Query, ev.cfg.Recall.MaxChars)
	row.dropped = len(picked) - len(packed.Keys)
	row.chars = utf8.RuneCountInString(packed.Text)
	seen := map[string]bool{}
	for _, k := range packed.Keys {
		row.injected++
		isLog := byKey[k].Kind == store.Log
		if isLog {
			row.injLog++
		}
		slug := ev.keySlug[k]
		if slug != "" && !seen[slug] && slices.Contains(q.gold, slug) {
			seen[slug] = true
			row.goldInj++
			if isLog {
				row.goldInjLog++
			}
		}
	}
	return row
}

func aggregate(rows []heldRow) LGResult {
	var res LGResult
	var goldChars, negChars float64
	for _, r := range rows {
		res.Records += r.injected
		res.LogRecords += r.injLog
		res.Dropped += r.dropped
		if r.injLog > 0 {
			res.LogPrompts++
		}
		if r.set == SetGold {
			res.Gold++
			goldChars += float64(r.chars)
			if r.goldN > 0 {
				res.Recall += float64(r.goldInj) / float64(r.goldN)
			}
			if r.goldInj > 0 {
				res.Hit++
			}
		} else {
			res.Negatives++
			negChars += float64(r.chars)
			if r.injected > 0 {
				res.FIR++
			}
		}
	}
	if res.Gold > 0 {
		n := float64(res.Gold)
		res.Recall, res.Hit, res.GoldChars = res.Recall/n, res.Hit/n, goldChars/n
	}
	if res.Negatives > 0 {
		n := float64(res.Negatives)
		res.FIR, res.NegChars = res.FIR/n, negChars/n
	}
	if res.Records > 0 {
		res.LogShare = float64(res.LogRecords) / float64(res.Records)
	}
	return res
}

func spreadLines(folds []LGFold) []string {
	if len(folds) > 0 && folds[0].LexGate != nil {
		var cov, sh, tp, ld []float64
		for _, f := range folds {
			cov, sh, tp = append(cov, f.LexGate.MinCover), append(sh, float64(f.LexGate.MinShared)), append(tp, float64(f.LexGate.MinTopic))
			if f.LexGate.Lead {
				ld = append(ld, float64(f.LexGate.LeadShared))
			}
		}
		return []string{fmt.Sprintf("lexical gate cover %s shared %s topic %s lead-shared %s (lead on in %d of %d folds)", spanOf(cov), spanOf(sh), spanOf(tp), spanOf(ld), len(ld), len(folds))}
	}
	rng := func(vals []float64, off bool) string {
		if len(vals) == 0 {
			return "-"
		}
		lo, hi := slices.Min(vals), slices.Max(vals)
		f := func(x float64) string {
			if off && x <= noMargin {
				return "off"
			}
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
		return f(lo) + ".." + f(hi)
	}
	var regCos, regMargin, regTake, logCos, logMargin, logTake []float64
	logsOff, lexical := 0, 0
	for _, f := range folds {
		if f.Lexical {
			lexical++
		} else if f.Regular.Take > 0 {
			regCos, regMargin, regTake = append(regCos, f.Regular.MinCos), append(regMargin, f.Regular.MinMargin), append(regTake, float64(f.Regular.Take))
		}
		if f.Log.Take > 0 {
			logCos, logMargin, logTake = append(logCos, f.Log.MinCos), append(logMargin, f.Log.MinMargin), append(logTake, float64(f.Log.Take))
		} else {
			logsOff++
		}
	}
	return []string{
		fmt.Sprintf("regular cos %s margin %s take %s (lexical gate in %d of %d folds)", rng(regCos, false), rng(regMargin, true), rng(regTake, false), lexical, len(folds)),
		fmt.Sprintf("log cos %s margin %s take %s (log injection off in %d of %d folds)", rng(logCos, false), rng(logMargin, true), rng(logTake, false), logsOff, len(folds)),
	}
}

func (ev *vecEval) logsGate(byKey map[string]store.Record, logSlug map[string]bool, gold, neg []*query, gres, nres []qres) (*LogsGateSummary, []heldRowSet, error) {
	cfg := ev.cfg
	k := cfg.Folds
	budgets := cfg.FIRBudgets
	lg := &LogsGateSummary{K: k, Budgets: budgets, Pools: cfg.Pools}
	var raw, negRaw [][]vh
	for i := range gres {
		raw = append(raw, gres[i].raw)
	}
	for i := range nres {
		negRaw = append(negRaw, nres[i].raw)
	}
	lg.Chunks = chunkStats(gold, raw, negRaw, logSlug)
	mk := func(qs []*query, rs []qres, pool string) []promptInput {
		out := make([]promptInput, len(qs))
		for i, q := range qs {
			p := promptInput{fold: foldK(q.Session, k), goldLen: len(q.gold), gold: q.gold}
			p.reg, p.logs = splitKinds(rs[i].pools[pool])
			for _, key := range rs[i].lexPick {
				p.lex = append(p.lex, vh{key: key, slug: ev.keySlug[key]})
			}
			if len(rs[i].lexSets) > 0 {
				p.lexGate = rs[i].lexGate
				for _, keys := range rs[i].lexSets {
					vhs := make([]vh, len(keys))
					for j, key := range keys {
						vhs[j] = vh{key: key, slug: ev.keySlug[key]}
					}
					p.lexSets = append(p.lexSets, vhs)
				}
			}
			out[i] = p
		}
		return out
	}
	var sets []heldRowSet
	lexChoice := regChoice{lexical: true}
	{
		gi, ni := mk(gold, gres, ""), mk(neg, nres, "")
		var rows []heldRow
		for i := range gi {
			rows = append(rows, ev.decideRow(&gi[i], gold[i], SetGold, lexChoice, KindGate{}, byKey))
		}
		for i := range ni {
			rows = append(rows, ev.decideRow(&ni[i], neg[i], SetNegative, lexChoice, KindGate{}, byKey))
		}
		lg.Lexical = aggregate(rows)
		lg.Lexical.Variant, lg.Lexical.Pool = lexicalVariant, "hook"
		sets = append(sets, heldRowSet{variant: lexicalVariant, budget: unsetBudget, rows: rows})
	}
	regGate, logGate := kindGrid(regTakes), kindGrid(logTakes)
	lg.RegGrid, lg.LogGrid = len(regGate), len(logGate)
	variants := lgVariants(cfg.Pools, cfg.LexRegular)
	if cfg.LexSweep {
		variants = append(variants, lgVariant{name: lexSweepVariant, lexSweep: true})
	}
	for _, v := range variants {
		gi, ni := mk(gold, gres, v.pool), mk(neg, nres, v.pool)
		l := newLayout(k, gi, ni)
		var regs []regChoice
		switch {
		case v.lexical:
			regs = []regChoice{lexChoice}
		case v.lexSweep:
			for i, g := range lexGrid() {
				regs = append(regs, regChoice{sweep: &i, lexGate: g})
			}
		default:
			for _, g := range regGate {
				regs = append(regs, regChoice{gate: g})
			}
		}
		logs := []KindGate{{MinMargin: noMargin}}
		if v.logs {
			logs = logGate
		}
		regStats := parStats(l, len(regs), func(i int) func(*promptInput) []string {
			return func(p *promptInput) []string { return slugsOf(regs[i].inject(p)) }
		}, ev.nw)
		logStats := parStats(l, len(logs), func(i int) func(*promptInput) []string {
			return func(p *promptInput) []string { return slugsOf(logs[i].apply(p.logs)) }
		}, ev.nw)
		picks := l.crossFit(regStats, logStats, budgets, ev.nw)
		for b, budget := range budgets {
			rows := make([]heldRow, 0, len(gi)+len(ni))
			for i := range gi {
				pk := picks[b][gi[i].fold]
				rows = append(rows, ev.decideRow(&gi[i], gold[i], SetGold, regs[pk.Reg], logs[pk.Log], byKey))
			}
			for i := range ni {
				pk := picks[b][ni[i].fold]
				rows = append(rows, ev.decideRow(&ni[i], neg[i], SetNegative, regs[pk.Reg], logs[pk.Log], byKey))
			}
			res := aggregate(rows)
			res.Variant, res.Pool, res.Budget = v.name, v.pool, budget
			for f := range k {
				pk := picks[b][f]
				var fr []heldRow
				for _, r := range rows {
					if r.fold == f {
						fr = append(fr, r)
					}
				}
				fa := aggregate(fr)
				fold := LGFold{Fold: f, Regular: regs[pk.Reg].gate, Lexical: regs[pk.Reg].lexical, Log: logs[pk.Log], Feasible: pk.Feasible,
					Gold: fa.Gold, Negatives: fa.Negatives, Recall: fa.Recall, FIR: fa.FIR}
				if regs[pk.Reg].sweep != nil {
					g := regs[pk.Reg].lexGate
					fold.LexGate = &g
				}
				if pk.Train.goldN > 0 {
					fold.TrainRecall = pk.Train.rec / float64(pk.Train.goldN)
				}
				if pk.Train.nN > 0 {
					fold.TrainFIR = float64(pk.Train.neg) / float64(pk.Train.nN)
				}
				if !pk.Feasible {
					res.Infeasible++
				}
				res.Folds = append(res.Folds, fold)
			}
			res.Spread = spreadLines(res.Folds)
			lg.Results = append(lg.Results, res)
			sets = append(sets, heldRowSet{variant: v.name, budget: strconv.FormatFloat(budget, 'g', -1, 64), rows: rows})
		}
	}
	return lg, sets, nil
}

type heldRowSet struct {
	variant string
	budget  string
	rows    []heldRow
}

func marginText(m float64) string {
	if m <= noMargin {
		return "off"
	}
	return fmt.Sprintf("%.2f", m)
}

func writeLogsGateFiles(out string, lg *LogsGateSummary, sets []heldRowSet) error {
	data, err := json.MarshalIndent(lg, "", "  ")
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(out, "logs-gate-summary.json")
	if err := os.WriteFile(jsonPath, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(jsonPath, 0o600); err != nil {
		return err
	}
	all := append([]LGResult{lg.Lexical}, lg.Results...)
	if err := writeFile(filepath.Join(out, "logs-gate-summary.tsv"), func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "variant\tbudget\tgold\tnegatives\trecall\thit\tfir\tchars_gold\tchars_negative\trecords\tlog_records\tlog_share\tlog_prompts\tdropped\tinfeasible_folds"); err != nil {
			return err
		}
		for _, r := range all {
			budget := unsetBudget
			if r.Variant != lexicalVariant {
				budget = strconv.FormatFloat(r.Budget, 'g', -1, 64)
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%.4f\t%.4f\t%.4f\t%.1f\t%.1f\t%d\t%d\t%.4f\t%d\t%d\t%d\n", r.Variant, budget, r.Gold, r.Negatives, r.Recall, r.Hit, r.FIR,
				r.GoldChars, r.NegChars, r.Records, r.LogRecords, r.LogShare, r.LogPrompts, r.Dropped, r.Infeasible); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(out, "logs-gate-picks.tsv"), func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "variant\tbudget\tfold\tregular_lexical\tregular_take\tregular_cos\tregular_margin\tlog_take\tlog_cos\tlog_margin\tfeasible\ttrain_recall\ttrain_fir\tgold\tnegatives\theld_out_recall\theld_out_fir\tlex_cover\tlex_shared\tlex_topic\tlex_lead_shared"); err != nil {
			return err
		}
		for _, r := range lg.Results {
			for _, f := range r.Folds {
				args := append([]any{r.Variant, strconv.FormatFloat(r.Budget, 'g', -1, 64), f.Fold,
					boolCol(f.Lexical), f.Regular.Take, f.Regular.MinCos, marginText(f.Regular.MinMargin), f.Log.Take, f.Log.MinCos, marginText(f.Log.MinMargin), boolCol(f.Feasible),
					f.TrainRecall, f.TrainFIR, f.Gold, f.Negatives, f.Recall, f.FIR}, lexCols(f.LexGate)...)
				if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%d\t%.2f\t%s\t%d\t%.2f\t%s\t%s\t%.4f\t%.4f\t%d\t%d\t%.4f\t%.4f\t%s\t%s\t%s\t%s\n", args...); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return writeFile(filepath.Join(out, "logs-gate-heldout.tsv"), func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "variant\tbudget\tset\tsession\tturn\thash\tprompt60\tfold\tgold_n\tinjected\tinjected_log\tgold_injected\tgold_injected_log\tdropped\tchars"); err != nil {
			return err
		}
		for _, s := range sets {
			for _, r := range s.rows {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", s.variant, s.budget, r.set, r.q.Session, r.q.Turn, PromptHash(r.q.Text), tsvField(promptPrefix(r.q.Text)),
					r.fold, r.goldN, r.injected, r.injLog, r.goldInj, r.goldInjLog, r.dropped, r.chars); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func printLogsGate(w io.Writer, lg *LogsGateSummary) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	c := lg.Chunks
	p("\nlogs gate: %d folds by session, FIR budgets %v, pools %s, grids %d regular x %d log settings\n", lg.K, lg.Budgets, strings.Join(lg.Pools, ","), lg.RegGrid, lg.LogGrid)
	p("gold prompts with a log-only gold file %d; its chunk in the raw top-12 for %d, two or more chunks %d, three or more %d, mean chunks %.2f\n",
		c.GoldLogPrompts, c.GoldInPool, c.GoldMulti2, c.GoldMulti3, c.GoldMeanChunks)
	p("raw top-1 is a log chunk: gold prompts %d (file repeats in top-12: %d), negatives %d (repeats: %d)\n", c.TopLogGold, c.TopLogRepeatGold, c.TopLogNeg, c.TopLogRepeatNeg)
	p("%-14s %7s %7s %7s %7s %9s %9s %7s %5s\n", "variant", "budget", "recall", "hit", "FIR", "chars/g", "chars/n", "log", "infe")
	row := func(r LGResult, budget string) {
		p("%-14s %7s %7.4f %7.4f %7.4f %9.1f %9.1f %7.3f %5d\n", r.Variant, budget, r.Recall, r.Hit, r.FIR, r.GoldChars, r.NegChars, r.LogShare, r.Infeasible)
	}
	row(lg.Lexical, unsetBudget)
	for _, r := range lg.Results {
		row(r, strconv.FormatFloat(r.Budget, 'g', -1, 64))
		for _, s := range r.Spread {
			p("      %s\n", s)
		}
	}
}

func logsGateFlags(fs *flag.FlagSet, cfg *Config, budgets, pools string) error {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if !cfg.LogsGate {
		for _, name := range []string{"folds", "fir-budgets", "pools", "lex-regular", "lex-sweep"} {
			if set[name] {
				return fmt.Errorf("--%s works only with --logs-gate", name)
			}
		}
		return nil
	}
	if cfg.Vectors == "" {
		return errors.New("--logs-gate needs --vectors")
	}
	if !cfg.LexicalRef {
		return errors.New("--logs-gate needs --lexical-ref, the shipped gate is its reference")
	}
	if cfg.Folds < 2 {
		return errors.New("--folds must be at least 2")
	}
	cfg.FIRBudgets = nil
	for _, s := range strings.Split(budgets, ",") {
		b, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || !(b >= 0 && b <= 1) {
			return fmt.Errorf("--fir-budgets: %q is not a number from 0 to 1", s)
		}
		if !slices.Contains(cfg.FIRBudgets, b) {
			cfg.FIRBudgets = append(cfg.FIRBudgets, b)
		}
	}
	cfg.Pools = nil
	for _, s := range strings.Split(pools, ",") {
		s = strings.TrimSpace(s)
		if !slices.Contains(AllPools, s) {
			return fmt.Errorf("--pools: %q is not one of %s", s, strings.Join(AllPools, ", "))
		}
		if !slices.Contains(cfg.Pools, s) {
			cfg.Pools = append(cfg.Pools, s)
		}
	}
	return nil
}

func spanOf(vals []float64) string {
	if len(vals) == 0 {
		return "-"
	}
	return strconv.FormatFloat(slices.Min(vals), 'f', -1, 64) + ".." + strconv.FormatFloat(slices.Max(vals), 'f', -1, 64)
}

func lexCols(g *recall.Gate) []any {
	if g == nil {
		return []any{"-", "-", "-", "-"}
	}
	lead := "-"
	if g.Lead {
		lead = strconv.Itoa(g.LeadShared)
	}
	return []any{fmt.Sprintf("%.2f", g.MinCover), strconv.Itoa(g.MinShared), strconv.Itoa(g.MinTopic), lead}
}

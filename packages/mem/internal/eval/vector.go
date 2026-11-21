package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

const (
	DefaultVecWeight = 1.0
	DefaultVecDepth  = 60
	noMargin         = -3.0
	unitTolerance    = 1e-3
	rankCut          = 10
)

var (
	vecSources   = []string{"vector", "fused"}
	vecTakes     = []int{1, recall.DefaultLimit}
	marginValues = []float64{noMargin, 0, 0.01, 0.02, 0.03, 0.05, 0.08, 0.12}
)

type vh struct {
	key  string
	slug string
	cos  float64
	log  bool
}

type fusedHit struct {
	key   string
	score float64
	bm25  float64
}

func fuse(words, tri []store.ListHit, vecKeys []string, w float64, limit int) []fusedHit {
	type acc struct {
		wr, tr, vr int
		bm         float64
	}
	by := map[string]*acc{}
	get := func(k string) *acc {
		a := by[k]
		if a == nil {
			a = &acc{}
			by[k] = a
		}
		return a
	}
	for _, h := range words {
		a := get(h.Key)
		a.wr = h.Rank
		a.bm += h.Score
	}
	for _, h := range tri {
		a := get(h.Key)
		a.tr = h.Rank
		a.bm += h.Score
	}
	if w > 0 {
		for i, k := range vecKeys {
			get(k).vr = i + 1
		}
	}
	out := make([]fusedHit, 0, len(by))
	for k, a := range by {
		var s float64
		if a.wr > 0 {
			s += 1 / float64(store.RRFK+a.wr)
		}
		if a.tr > 0 {
			s += store.TriWeight / float64(store.RRFK+a.tr)
		}
		if a.vr > 0 {
			s += w / float64(store.RRFK+a.vr)
		}
		out = append(out, fusedHit{key: k, score: s, bm25: a.bm})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.bm25 != b.bm25 {
			return a.bm25 > b.bm25
		}
		return a.key < b.key
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func gateInject(list []vh, take int, minCos, minMargin float64) []string {
	var out []string
	for _, h := range (KindGate{Take: take, MinCos: minCos, MinMargin: minMargin}).apply(list) {
		out = append(out, h.key)
	}
	return out
}

type VecSetting struct {
	Source    string  `json:"source"`
	Take      int     `json:"take"`
	MinCos    float64 `json:"min_cos"`
	MinMargin float64 `json:"min_margin"`
}

func VecGrid() []VecSetting {
	var out []VecSetting
	for _, src := range vecSources {
		for _, take := range vecTakes {
			for c := 20; c <= 95; c++ {
				for _, m := range marginValues {
					out = append(out, VecSetting{Source: src, Take: take, MinCos: float64(c) / 100, MinMargin: m})
				}
			}
		}
	}
	return out
}

type VecRow struct {
	VecSetting
	Gold       HookStats  `json:"gold"`
	Negative   HookStats  `json:"negative"`
	Folds      [2]Fold    `json:"folds"`
	Utility    [2]float64 `json:"fold_utility"`
	Worst      float64    `json:"worst_fold_utility"`
	WorstBest  bool       `json:"worst_fold_best"`
	Pareto     bool       `json:"pareto"`
	Knee       bool       `json:"knee"`
	FoldPareto [2]bool    `json:"fold_pareto"`
	FoldKnee   [2]bool    `json:"fold_knee"`
}

func frontierMax(pts []point, pareto []bool) (float64, float64) {
	var maxR, maxF float64
	for i, p := range pts {
		if pareto[i] {
			maxR, maxF = max(maxR, p.r), max(maxF, p.f)
		}
	}
	return maxR, maxF
}

func utility(p point, maxR, maxF float64) float64 {
	var u float64
	if maxR > 0 {
		u += p.r / maxR
	}
	if maxF > 0 {
		u -= p.f / maxF
	}
	return u
}

func markWorstFold(rows []VecRow) int {
	for f := range 2 {
		pts := make([]point, len(rows))
		for i, r := range rows {
			pts[i] = point{r.Folds[f].Recall, r.Folds[f].FalseRate}
		}
		pareto, _ := frontier(pts)
		maxR, maxF := frontierMax(pts, pareto)
		for i := range rows {
			rows[i].Utility[f] = utility(pts[i], maxR, maxF)
		}
	}
	best := -1
	for i := range rows {
		rows[i].Worst = math.Min(rows[i].Utility[0], rows[i].Utility[1])
		if best < 0 || rows[i].Worst > rows[best].Worst {
			best = i
		}
	}
	if best >= 0 {
		rows[best].WorstBest = true
	}
	return best
}

func vecKnee(pts []point, pareto []bool) int {
	maxR, maxF := frontierMax(pts, pareto)
	best, bestV := -1, math.Inf(-1)
	for i, p := range pts {
		if !pareto[i] {
			continue
		}
		if v := utility(p, maxR, maxF); v > bestV || v == bestV && p.f < pts[best].f {
			best, bestV = i, v
		}
	}
	return best
}

func markVecFrontier(rows []VecRow) {
	pts := make([]point, len(rows))
	for i, r := range rows {
		pts[i] = point{r.Gold.Recall, r.Negative.InjectRate}
	}
	pareto, _ := frontier(pts)
	knee := vecKnee(pts, pareto)
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
		pareto, _ := frontier(pts)
		knee := vecKnee(pts, pareto)
		for i := range rows {
			rows[i].FoldPareto[f] = pareto[i]
		}
		if knee >= 0 {
			rows[knee].FoldKnee[f] = true
		}
	}
}

type VecCrossFit struct {
	VecSetting
	PickedOn  int        `json:"picked_on_fold"`
	InFold    Fold       `json:"in_fold"`
	HeldOut   Fold       `json:"held_out"`
	HeldKnee  VecSetting `json:"held_out_knee"`
	HeldBest  Fold       `json:"held_out_knee_stats"`
	HeldAtFIR float64    `json:"held_out_best_recall_at_same_fir"`
}

func crossFitVec(rows []VecRow) []VecCrossFit {
	knee := func(f int) int {
		for i, r := range rows {
			if r.FoldKnee[f] {
				return i
			}
		}
		return -1
	}
	var out []VecCrossFit
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
		out = append(out, VecCrossFit{VecSetting: rows[a].VecSetting, PickedOn: f, InFold: rows[a].Folds[f], HeldOut: held,
			HeldKnee: rows[b].VecSetting, HeldBest: rows[b].Folds[1-f], HeldAtFIR: best})
	}
	return out
}

type VecMetric struct {
	Source     string  `json:"source"`
	Gold       string  `json:"gold"`
	Prompts    int     `json:"prompts"`
	Cand12Hits int     `json:"candidate_hits"`
	Cand12     float64 `json:"candidate_recall"`
	Recall12   float64 `json:"recall_at_pool"`
	Hit4       float64 `json:"hit_at_4"`
	Recall4    float64 `json:"recall_at_4"`
	MRR10      float64 `json:"mrr_at_10"`
}

type VecCalibration struct {
	Source     string    `json:"source"`
	TopGold    Quantiles `json:"top1_cos_gold"`
	TopNeg     Quantiles `json:"top1_cos_negative"`
	MarginGold Quantiles `json:"margin_gold"`
	MarginNeg  Quantiles `json:"margin_negative"`
}

type VecLexGate struct {
	Gold     HookStats `json:"gold"`
	Negative HookStats `json:"negative"`
	Folds    [2]Fold   `json:"folds"`
}

type VecGroup struct {
	Source       string        `json:"source"`
	Take         int           `json:"take"`
	Knee         *VecRow       `json:"knee,omitempty"`
	WorstBest    *VecRow       `json:"worst_fold_best,omitempty"`
	AtLexicalFIR *VecRow       `json:"best_recall_at_lexical_fir,omitempty"`
	CrossFit     []VecCrossFit `json:"crossfit,omitempty"`
}

type VecCaseHit struct {
	Key string  `json:"key"`
	Cos float64 `json:"cos"`
}

type VecCase struct {
	Name       string       `json:"name"`
	Want       []string     `json:"want,omitempty"`
	None       bool         `json:"none,omitempty"`
	Vector     []VecCaseHit `json:"vector"`
	Fused      []VecCaseHit `json:"fused"`
	LexInjects []string     `json:"lexical_injected"`
}

type VectorSummary struct {
	Model          string           `json:"model"`
	Dim            int              `json:"dim"`
	QueryVectors   string           `json:"query_vectors"`
	DocsVectors    string           `json:"docs_vectors"`
	DB             string           `json:"db"`
	Records        int              `json:"records"`
	Eligible       int              `json:"eligible_records"`
	Gold           int              `json:"gold_prompts"`
	Negatives      int              `json:"negative_prompts"`
	CaseCount      int              `json:"case_prompts"`
	Pool           int              `json:"pool"`
	VecWeight      float64          `json:"vec_weight"`
	VecDepth       int              `json:"vec_depth"`
	FusionMismatch int              `json:"fusion_mismatch"`
	Metrics        []VecMetric      `json:"metrics"`
	Calibration    []VecCalibration `json:"calibration"`
	LexicalGate    VecLexGate       `json:"lexical_gate"`
	Groups         []VecGroup       `json:"groups"`
	Cases          []VecCase        `json:"cases,omitempty"`
	Rows           []VecRow         `json:"rows"`
	LogsGate       *LogsGateSummary `json:"logs_gate,omitempty"`
	GoldSet        *GoldReport      `json:"gold_set,omitempty"`
	Seconds        float64          `json:"seconds"`
}

func (s VectorSummary) Row(source string, take int, minCos, minMargin float64) (VecRow, bool) {
	for _, r := range s.Rows {
		if r.Source == source && r.Take == take && math.Abs(r.MinCos-minCos) < 1e-9 && math.Abs(r.MinMargin-minMargin) < 1e-9 {
			return r, true
		}
	}
	return VecRow{}, false
}

type qres struct {
	lex, vec, fus, all []vh
	lexOut             outcome
	mismatch           bool
	pools              map[string][]vh
	raw                []vh
	lexPick            []string
	lexSets            [][]string
	lexGate            []uint16
}

type vecEval struct {
	s        *store.Store
	cfg      Config
	docs     *vec.Set
	queries  *vec.Set
	qIndex   map[string]int
	index    map[string]int
	keySlug  map[string]string
	isLog    []bool
	elig     []int
	eligAll  []int
	pool     int
	settings recall.Settings
	nw       int
}

func (ev *vecEval) vh(i int, cos []float64) vh {
	k := ev.docs.Names[i]
	return vh{key: k, slug: ev.keySlug[k], cos: cos[i], log: ev.isLog[i]}
}

func (ev *vecEval) order(cos []float64, idx []int) []int {
	out := slices.Clone(idx)
	slices.SortFunc(out, func(a, b int) int {
		if cos[a] != cos[b] {
			if cos[a] > cos[b] {
				return -1
			}
			return 1
		}
		return strings.Compare(ev.docs.Names[a], ev.docs.Names[b])
	})
	return out
}

func (ev *vecEval) dedupe(ordered []int, cos []float64, n int) []vh {
	seen := map[string]bool{}
	var out []vh
	for _, i := range ordered {
		h := ev.vh(i, cos)
		id := h.slug
		if id == "" {
			id = h.key
		}
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

func (ev *vecEval) one(ctx context.Context, q *query, wantAll bool) (qres, error) {
	var res qres
	row := ev.queries.Row(ev.qIndex[queryID(q)])
	cos := make([]float64, len(ev.docs.Names))
	for i := range cos {
		cos[i] = vec.Dot(row, ev.docs.Row(i))
	}
	ordered := ev.order(cos, ev.elig)
	res.vec = make([]vh, 0, ev.pool)
	for _, i := range ordered[:min(ev.pool, len(ordered))] {
		res.vec = append(res.vec, ev.vh(i, cos))
	}
	vkeys := make([]string, 0, ev.cfg.VecDepth)
	for _, i := range ordered[:min(ev.cfg.VecDepth, len(ordered))] {
		vkeys = append(vkeys, ev.docs.Names[i])
	}
	words, tri, err := ev.s.Lists(ctx, recall.SearchQuery(q.Query, ev.pool, nil, nil))
	if err != nil {
		return res, err
	}
	toVH := func(in []fusedHit) []vh {
		out := make([]vh, len(in))
		for i, f := range in {
			out[i] = ev.vh(ev.index[f.key], cos)
		}
		return out
	}
	res.lex = toVH(fuse(words, tri, nil, 0, ev.pool))
	res.fus = toVH(fuse(words, tri, vkeys, ev.cfg.VecWeight, ev.pool))
	var allOrdered []int
	if wantAll || ev.cfg.LogsGate {
		allOrdered = ev.order(cos, ev.eligAll)
	}
	if wantAll {
		res.all = ev.dedupe(allOrdered, cos, ev.pool)
	}
	if ev.cfg.LogsGate {
		ev.poolLists(&res, cos, ordered, allOrdered, words, tri)
	}
	if ev.cfg.LexicalRef {
		hits, err := ev.s.Search(ctx, recall.SearchQuery(q.Query, ev.pool, nil, nil))
		if err != nil {
			return res, err
		}
		if len(hits) != len(res.lex) {
			res.mismatch = true
		} else {
			for i, h := range hits {
				if h.Record.Key != res.lex[i].key {
					res.mismatch = true
				}
			}
		}
		inj := ev.settings.Inject(q.Query, recall.Candidates(q.Query, hits))
		res.lexOut = outcomeOf(inj)
		for _, c := range inj.Picks {
			res.lexPick = append(res.lexPick, c.Hit.Record.Key)
		}
		if ev.cfg.LexSweep {
			res.lexSets, res.lexGate = sweepPicks(ev.settings, recall.Candidates(q.Query, hits))
		}
	}
	return res, nil
}

func queryID(q *query) string {
	if q.Session == CaseSession {
		return CaseSession + ":" + q.caseName
	}
	return fmt.Sprintf("%s:%d", q.Session, q.Turn)
}

type listScore struct {
	best  int
	n4, n int
	golds int
}

func scoreList(list []vh, gold []string) listScore {
	sc := listScore{golds: len(gold)}
	seen := map[string]bool{}
	for i, h := range list {
		if h.slug == "" || seen[h.slug] || !slices.Contains(gold, h.slug) {
			continue
		}
		seen[h.slug] = true
		sc.n++
		if i < 4 {
			sc.n4++
		}
		if sc.best == 0 {
			sc.best = i + 1
		}
	}
	return sc
}

func vecMetrics(source, goldSet string, qs []*query, lists [][]vh) VecMetric {
	m := VecMetric{Source: source, Gold: goldSet}
	for i, q := range qs {
		gold := q.gold
		if goldSet != "read" {
			gold = unwritten(q)
		}
		if len(gold) == 0 {
			continue
		}
		sc := scoreList(lists[i], gold)
		m.Prompts++
		if sc.best > 0 {
			m.Cand12Hits++
			m.Recall12 += float64(sc.n) / float64(len(gold))
		}
		if sc.n4 > 0 {
			m.Hit4++
		}
		m.Recall4 += float64(sc.n4) / float64(len(gold))
		if sc.best > 0 && sc.best <= rankCut {
			m.MRR10 += 1 / float64(sc.best)
		}
	}
	if m.Prompts > 0 {
		n := float64(m.Prompts)
		m.Cand12 = float64(m.Cand12Hits) / n
		m.Recall12, m.Hit4, m.Recall4, m.MRR10 = m.Recall12/n, m.Hit4/n, m.Recall4/n, m.MRR10/n
	}
	return m
}

func topCos(list []vh) (float64, float64) {
	if len(list) == 0 {
		return 0, 0
	}
	second := 0.0
	if len(list) > 1 {
		second = list[1].cos
	}
	return list[0].cos, list[0].cos - second
}

func calibration(source string, gold, neg [][]vh) VecCalibration {
	collect := func(sets [][]vh) ([]float64, []float64) {
		var tops, margins []float64
		for _, l := range sets {
			t, m := topCos(l)
			tops, margins = append(tops, t), append(margins, m)
		}
		return tops, margins
	}
	gt, gm := collect(gold)
	nt, nm := collect(neg)
	return VecCalibration{Source: source, TopGold: quantiles(gt), TopNeg: quantiles(nt), MarginGold: quantiles(gm), MarginNeg: quantiles(nm)}
}

func checkNames(what string, got, want []string, allowExtra bool) error {
	gs, ws := map[string]bool{}, map[string]bool{}
	for _, g := range got {
		gs[g] = true
	}
	for _, w := range want {
		ws[w] = true
	}
	var missing, extra []string
	for w := range ws {
		if !gs[w] {
			missing = append(missing, w)
		}
	}
	for g := range gs {
		if !ws[g] {
			extra = append(extra, g)
		}
	}
	if len(missing) == 0 && (len(extra) == 0 || allowExtra) {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	show := func(in []string) string {
		if len(in) > 3 {
			return fmt.Sprintf("%s and %d more", strings.Join(in[:3], ", "), len(in)-3)
		}
		return strings.Join(in, ", ")
	}
	return fmt.Errorf("%s keys do not match: %d vectors missing for [%s], %d vectors without a counterpart [%s]", what, len(missing), show(missing), len(extra), show(extra))
}

func checkUnit(what string, s *vec.Set) error {
	for i := range s.Names {
		var n float64
		for _, x := range s.Row(i) {
			n += float64(x) * float64(x)
		}
		if math.IsNaN(n) || math.Abs(n-1) > unitTolerance {
			return fmt.Errorf("%s %s: vector is not unit length (squared norm %.4f)", what, s.Names[i], n)
		}
	}
	return nil
}

func EvaluateVectors(ctx context.Context, e env.Env, cfg Config) (VectorSummary, error) {
	var sum VectorSummary
	p, err := prepare(ctx, e, cfg)
	if err != nil {
		return sum, err
	}
	defer func() { _ = p.s.Close() }()
	docsDir := cfg.DocsVectors
	if docsDir == "" {
		docsDir = cfg.Vectors
	}
	meta, err := vec.ReadMeta(cfg.Vectors)
	if err != nil {
		return sum, err
	}
	queries, err := vec.Load(cfg.Vectors, vec.PartQueries)
	if err != nil {
		return sum, err
	}
	docs, err := vec.Load(docsDir, vec.PartDocs)
	if err != nil {
		return sum, err
	}
	docsMeta := meta
	if docsDir != cfg.Vectors {
		if docsMeta, err = vec.ReadMeta(docsDir); err != nil {
			return sum, err
		}
		if docsMeta.Model != meta.Model {
			return sum, fmt.Errorf("document vectors come from model %q, query vectors from %q", docsMeta.Model, meta.Model)
		}
	}
	if docs.Dim != queries.Dim {
		return sum, fmt.Errorf("document vectors have dimension %d, query vectors %d", docs.Dim, queries.Dim)
	}
	recs, err := p.s.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return sum, err
	}
	keys := make([]string, len(recs))
	byKey := make(map[string]store.Record, len(recs))
	for i, r := range recs {
		keys[i], byKey[r.Key] = r.Key, r
	}
	if err := checkNames("docs", docs.Names, keys, false); err != nil {
		return sum, err
	}
	var want []string
	var gold, neg, cases []*query
	for _, q := range p.qs {
		want, gold = append(want, p.id(q)), append(gold, q)
	}
	for _, q := range p.negs {
		want, neg = append(want, p.id(q)), append(neg, q)
	}
	for _, q := range p.cases {
		want, cases = append(want, p.id(q)), append(cases, q)
	}
	if err := checkNames("queries", queries.Names, want, cfg.AllowExtraVectors); err != nil {
		return sum, err
	}
	if !cfg.AllowOtherTexts {
		if err := p.checkExport(ctx, meta, docsMeta); err != nil {
			return sum, err
		}
	}
	if err := checkUnit("document", docs); err != nil {
		return sum, err
	}
	if err := checkUnit("query", queries); err != nil {
		return sum, err
	}
	ev := &vecEval{s: p.s, cfg: cfg, docs: docs, queries: queries, qIndex: map[string]int{}, index: map[string]int{}, keySlug: p.keySlug, isLog: make([]bool, len(docs.Names)), pool: cfg.Recall.Pool, settings: cfg.Recall, nw: p.nw}
	for i, n := range queries.Names {
		ev.qIndex[n] = i
	}
	for i, n := range docs.Names {
		ev.index[n] = i
		r := byKey[n]
		ev.isLog[i] = r.Kind == store.Log
		if r.Status == store.Current {
			ev.eligAll = append(ev.eligAll, i)
			if r.Kind != store.Log {
				ev.elig = append(ev.elig, i)
			}
		}
	}
	if len(ev.elig) == 0 {
		return sum, fmt.Errorf("no eligible records: all %d are logs or not current", len(recs))
	}
	start := time.Now()
	type job struct {
		q   *query
		out *qres
		all bool
	}
	var jobs []job
	results := func(qs []*query, all bool) []qres {
		out := make([]qres, len(qs))
		for i, q := range qs {
			jobs = append(jobs, job{q, &out[i], all})
		}
		return out
	}
	gres, nres, cres := results(gold, true), results(neg, false), results(cases, false)
	ch := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for range p.nw {
		wg.Go(func() {
			for j := range ch {
				r, err := ev.one(ctx, j.q, j.all)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				*j.out = r
			}
		})
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	if firstErr != nil {
		return sum, firstErr
	}
	sum = VectorSummary{Model: meta.Model, Dim: docs.Dim, QueryVectors: cfg.Vectors, DocsVectors: docsDir, DB: p.sum.DB, Records: len(recs), Eligible: len(ev.elig),
		Gold: len(gold), Negatives: len(neg), CaseCount: len(cases), Pool: ev.pool, VecWeight: cfg.VecWeight, VecDepth: cfg.VecDepth, GoldSet: p.gold}
	lists := func(res []qres, pick func(qres) []vh) [][]vh {
		out := make([][]vh, len(res))
		for i, r := range res {
			out[i] = pick(r)
		}
		return out
	}
	srcs := []struct {
		name string
		pick func(qres) []vh
	}{
		{"lexical", func(r qres) []vh { return r.lex }},
		{"vector", func(r qres) []vh { return r.vec }},
		{"fused", func(r qres) []vh { return r.fus }},
		{"vector-all", func(r qres) []vh { return r.all }},
	}
	for _, goldSet := range []string{"read", "read-not-written"} {
		for _, s := range srcs {
			sum.Metrics = append(sum.Metrics, vecMetrics(s.name, goldSet, gold, lists(gres, s.pick)))
		}
	}
	for _, s := range srcs[1:3] {
		sum.Calibration = append(sum.Calibration, calibration(s.name, lists(gres, s.pick), lists(nres, s.pick)))
	}
	for _, rs := range [][]qres{gres, nres, cres} {
		for _, r := range rs {
			if r.mismatch {
				sum.FusionMismatch++
			}
		}
	}
	if cfg.LexicalRef {
		gout, nout := make([]outcome, len(gres)), make([]outcome, len(nres))
		for i, r := range gres {
			gout[i] = r.lexOut
		}
		for i, r := range nres {
			nout[i] = r.lexOut
		}
		sum.LexicalGate = VecLexGate{Gold: hookStats(SetGold, gold, gout, p.keySlug), Negative: hookStats(SetNegative, neg, nout, p.keySlug), Folds: folds(gold, gout, neg, nout, p.keySlug)}
	}
	grid := VecGrid()
	sum.Rows = make([]VecRow, len(grid))
	rowJobs := make(chan int)
	var rwg sync.WaitGroup
	for range p.nw {
		rwg.Go(func() {
			for gi := range rowJobs {
				st := grid[gi]
				pick := srcs[1].pick
				if st.Source == "fused" {
					pick = srcs[2].pick
				}
				gout, nout := make([]outcome, len(gres)), make([]outcome, len(nres))
				for i, r := range gres {
					gout[i] = outcome{keys: gateInject(pick(r), st.Take, st.MinCos, st.MinMargin)}
				}
				for i, r := range nres {
					nout[i] = outcome{keys: gateInject(pick(r), st.Take, st.MinCos, st.MinMargin)}
				}
				sum.Rows[gi] = VecRow{VecSetting: st, Gold: hookStats(SetGold, gold, gout, p.keySlug), Negative: hookStats(SetNegative, neg, nout, p.keySlug),
					Folds: folds(gold, gout, neg, nout, p.keySlug)}
			}
		})
	}
	for gi := range grid {
		rowJobs <- gi
	}
	close(rowJobs)
	rwg.Wait()
	for _, src := range vecSources {
		for _, take := range vecTakes {
			var idx []int
			for i, r := range sum.Rows {
				if r.Source == src && r.Take == take {
					idx = append(idx, i)
				}
			}
			group := make([]VecRow, len(idx))
			for k, i := range idx {
				group[k] = sum.Rows[i]
			}
			markVecFrontier(group)
			markWorstFold(group)
			for k, i := range idx {
				sum.Rows[i] = group[k]
			}
			g := VecGroup{Source: src, Take: take, CrossFit: crossFitVec(group)}
			var atFIR *VecRow
			for k := range group {
				r := group[k]
				if r.Knee {
					g.Knee = &r
				}
				if r.WorstBest {
					g.WorstBest = &r
				}
				if cfg.LexicalRef && r.Negative.InjectRate <= sum.LexicalGate.Negative.InjectRate && (atFIR == nil || r.Gold.Recall > atFIR.Gold.Recall) {
					atFIR = &group[k]
				}
			}
			g.AtLexicalFIR = atFIR
			sum.Groups = append(sum.Groups, g)
		}
	}
	for i, c := range p.caseSpecs {
		top := func(l []vh) []VecCaseHit {
			var out []VecCaseHit
			for _, h := range l[:min(4, len(l))] {
				out = append(out, VecCaseHit{Key: h.key, Cos: h.cos})
			}
			return out
		}
		inj := cres[i].lexOut.keys
		if inj == nil {
			inj = []string{}
		}
		sum.Cases = append(sum.Cases, VecCase{Name: c.Name, Want: c.Want, None: c.None, Vector: top(cres[i].vec), Fused: top(cres[i].fus), LexInjects: inj})
	}
	var lgSets []heldRowSet
	if cfg.LogsGate {
		if sum.LogsGate, lgSets, err = ev.logsGate(byKey, p.logSlug, gold, neg, gres, nres); err != nil {
			return sum, err
		}
	}
	sum.Seconds = time.Since(start).Seconds()
	if err := writeVectorFiles(cfg.Out, sum, gold, neg, gres, nres); err != nil {
		return sum, err
	}
	if cfg.LogsGate {
		if err := writeLogsGateFiles(cfg.Out, sum.LogsGate, lgSets); err != nil {
			return sum, err
		}
	}
	return sum, nil
}

func (p *prepared) checkExport(ctx context.Context, queryMeta, docsMeta *vec.Meta) error {
	qt, err := p.queryTexts()
	if err != nil {
		return err
	}
	dt, err := p.docTexts(ctx)
	if err != nil {
		return err
	}
	qsum, err := sumJSONL(qt)
	if err != nil {
		return err
	}
	dsum, err := sumJSONL(dt)
	if err != nil {
		return err
	}
	for _, c := range []struct {
		what, now string
		part      *vec.PartMeta
	}{{"query", qsum, queryMeta.Queries}, {"document", dsum, docsMeta.Docs}} {
		if c.part != nil && c.part.InputSHA256 != "" && c.part.InputSHA256 != c.now {
			return fmt.Errorf("%s vectors were made from a different export than this run produces (input sha256 %.12s, now %.12s); export and embed again or pass --allow-other-texts", c.what, c.part.InputSHA256, c.now)
		}
	}
	return nil
}

func bestRank(l []vh, gold []string) int {
	return scoreList(l, gold).best
}

func writeVectorFiles(out string, sum VectorSummary, gold, neg []*query, gres, nres []qres) error {
	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "vector-summary.json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(out, "vector-summary.json"), 0o600); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(out, "vector-summary.tsv"), func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "source\tgold\tprompts\tcandidate_hits\tcandidate_recall\trecall_at_pool\thit_at_4\trecall_at_4\tmrr_at_10"); err != nil {
			return err
		}
		for _, m := range sum.Metrics {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\n", m.Source, m.Gold, m.Prompts, m.Cand12Hits, m.Cand12, m.Recall12, m.Hit4, m.Recall4, m.MRR10); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(out, "vector-sweep.tsv"), func(w *bufio.Writer) error {
		head := "source\ttake\tmin_cos\tmin_margin\tgold_recall\tgold_hit\tgold_rate\tgold_precision\tgold_records\tneg_fir\tneg_records\trecall_fold0\tfir_fold0\trecall_fold1\tfir_fold1\t" +
			"utility_fold0\tutility_fold1\tworst_fold_utility\tworst_fold_best\tpareto\tknee\tpareto_fold0\tknee_fold0\tpareto_fold1\tknee_fold1"
		if _, err := fmt.Fprintln(w, head); err != nil {
			return err
		}
		for _, r := range sum.Rows {
			margin := "off"
			if r.MinMargin > noMargin {
				margin = fmt.Sprintf("%.2f", r.MinMargin)
			}
			g, n := r.Gold, r.Negative
			_, err := fmt.Fprintf(w, "%s\t%d\t%.2f\t%s\t%.4f\t%.4f\t%.4f\t%.4f\t%.3f\t%.4f\t%.3f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				r.Source, r.Take, r.MinCos, margin, g.Recall, g.Hit, g.InjectRate, g.Precision, g.MeanRecords, n.InjectRate, n.MeanRecords,
				r.Folds[0].Recall, r.Folds[0].FalseRate, r.Folds[1].Recall, r.Folds[1].FalseRate, r.Utility[0], r.Utility[1], r.Worst,
				boolCol(r.WorstBest), boolCol(r.Pareto), boolCol(r.Knee), boolCol(r.FoldPareto[0]), boolCol(r.FoldKnee[0]), boolCol(r.FoldPareto[1]), boolCol(r.FoldKnee[1]))
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return writeFile(filepath.Join(out, "vector-queries.tsv"), func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "set\tsession\tturn\thash\tprompt60\tgold\tlexical_rank\tvector_rank\tfused_rank\tvector_all_rank\tvector_cos1\tvector_cos2\tvector_key1\tfused_cos1\tfused_cos2\tfused_key1\tgold_n\tlexical_n4\tvector_n4\tfused_n4\tvector_all_n4"); err != nil {
			return err
		}
		emit := func(set string, qs []*query, rs []qres) error {
			for i, q := range qs {
				r := rs[i]
				v1, _ := topCos(r.vec)
				f1, _ := topCos(r.fus)
				v2, f2 := 0.0, 0.0
				if len(r.vec) > 1 {
					v2 = r.vec[1].cos
				}
				if len(r.fus) > 1 {
					f2 = r.fus[1].cos
				}
				key := func(l []vh) string {
					if len(l) == 0 {
						return ""
					}
					return l[0].key
				}
				_, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%.4f\t%.4f\t%s\t%.4f\t%.4f\t%s\t%d\t%d\t%d\t%d\t%d\n", set, q.Session, q.Turn, PromptHash(q.Text), tsvField(promptPrefix(q.Text)),
					strings.Join(q.gold, ","), bestRank(r.lex, q.gold), bestRank(r.vec, q.gold), bestRank(r.fus, q.gold), bestRank(r.all, q.gold), v1, v2, key(r.vec), f1, f2, key(r.fus), len(q.gold), scoreList(r.lex, q.gold).n4, scoreList(r.vec, q.gold).n4, scoreList(r.fus, q.gold).n4, scoreList(r.all, q.gold).n4)
				if err != nil {
					return err
				}
			}
			return nil
		}
		if err := emit(SetGold, gold, gres); err != nil {
			return err
		}
		return emit(SetNegative, neg, nres)
	})
}

func printVectorSummary(w io.Writer, sum VectorSummary) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	p("model %s, dim %d, %d records (%d eligible: current, not log), db %s\n", sum.Model, sum.Dim, sum.Records, sum.Eligible, sum.DB)
	p("queries %s, documents %s\n", sum.QueryVectors, sum.DocsVectors)
	if g := sum.GoldSet; g != nil {
		printGold(w, g)
	}
	p("gold prompts %d, negatives %d, cases %d; pool %d, vec weight %g, vec depth %d, %.1fs\n", sum.Gold, sum.Negatives, sum.CaseCount, sum.Pool, sum.VecWeight, sum.VecDepth, sum.Seconds)
	p("fusion check: %d queries where the weight-0 fusion differs from the store's ranking\n\n", sum.FusionMismatch)
	p("%-11s %-17s %7s %12s %8s %8s %7s %7s %8s\n", "source", "gold", "prompts", "candidates", "cand", "R@pool", "hit@4", "R@4", "MRR@10")
	for _, m := range sum.Metrics {
		p("%-11s %-17s %7d %6d/%-5d %8.3f %8.3f %7.3f %7.3f %8.3f\n", m.Source, m.Gold, m.Prompts, m.Cand12Hits, m.Prompts, m.Cand12, m.Recall12, m.Hit4, m.Recall4, m.MRR10)
	}
	q := func(x Quantiles) string { return fmt.Sprintf("%d, %.3f / %.3f / %.3f", x.N, x.P10, x.P50, x.P90) }
	p("\ntop-1 cosine and top1-top2 margin, n, p10 / p50 / p90\n")
	for _, c := range sum.Calibration {
		p("%s cos1 gold %s; negative %s\n", c.Source, q(c.TopGold), q(c.TopNeg))
		p("%s margin gold %s; negative %s\n", c.Source, q(c.MarginGold), q(c.MarginNeg))
	}
	lg := sum.LexicalGate
	if lg.Gold.Prompts > 0 {
		p("\nlexical hook gate (reference): gold recall %.4f hit %.4f injecting %d of %d, negatives injecting %d of %d (FIR %.4f)\n",
			lg.Gold.Recall, lg.Gold.Hit, lg.Gold.Injecting, lg.Gold.Prompts, lg.Negative.Injecting, lg.Negative.Prompts, lg.Negative.InjectRate)
	}
	gs := func(r VecRow) string {
		margin := "off"
		if r.MinMargin > noMargin {
			margin = fmt.Sprintf("%.2f", r.MinMargin)
		}
		return fmt.Sprintf("cos >= %.2f, margin %s", r.MinCos, margin)
	}
	for _, g := range sum.Groups {
		p("\ngate on %s, inject up to %d:\n", g.Source, g.Take)
		show := func(label string, r *VecRow) {
			if r != nil {
				p("  %-22s %s: recall %.4f hit %.4f, FIR %.4f; fold0 %.4f/%.4f fold1 %.4f/%.4f; worst-fold utility %.3f\n", label, gs(*r), r.Gold.Recall, r.Gold.Hit, r.Negative.InjectRate,
					r.Folds[0].Recall, r.Folds[0].FalseRate, r.Folds[1].Recall, r.Folds[1].FalseRate, r.Worst)
			}
		}
		show("knee over both folds", g.Knee)
		show("best worst-fold", g.WorstBest)
		show("best at lexical FIR", g.AtLexicalFIR)
		for _, cf := range g.CrossFit {
			margin := "off"
			if cf.MinMargin > noMargin {
				margin = fmt.Sprintf("%.2f", cf.MinMargin)
			}
			p("  knee of fold %d (cos >= %.2f, margin %s): recall %.4f, FIR %.4f there; on fold %d recall %.4f, FIR %.4f; best recall there at FIR <= %.4f: %.4f\n",
				cf.PickedOn, cf.MinCos, margin, cf.InFold.Recall, cf.InFold.FalseRate, 1-cf.PickedOn, cf.HeldOut.Recall, cf.HeldOut.FalseRate, cf.HeldOut.FalseRate, cf.HeldAtFIR)
		}
	}
	if sum.LogsGate != nil {
		printLogsGate(w, sum.LogsGate)
	}
	if len(sum.Cases) > 0 {
		p("\ncases (top-4 by cosine)\n")
		for _, c := range sum.Cases {
			want := "nothing"
			if !c.None {
				want = strings.Join(c.Want, ",")
			}
			p("%s: want %s\n", c.Name, want)
			for _, h := range c.Vector {
				p("     vector %s %.4f\n", h.Key, h.Cos)
			}
			for _, h := range c.Fused {
				p("     fused  %s %.4f\n", h.Key, h.Cos)
			}
		}
	}
}

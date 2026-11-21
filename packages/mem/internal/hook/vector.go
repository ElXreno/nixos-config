package hook

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/counter"
	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/statefile"
	"mem/internal/store"
	"mem/internal/vec"
)

const (
	CountersDir   = "counters"
	outcomeOK     = "ok"
	outcomeDim    = "dim"
	outcomeNoVecs = "novectors"
	outcomeError  = "error"
)

const (
	maxEmbedReply = 1 << 20
	embedSlack    = 25 * time.Millisecond
)

var errEmbedPanic = errors.New("embedder panicked")

var embedderFor = func(url, model string, timeout time.Duration) recall.Embedder {
	return vec.NewClient(vec.Options{URL: url, Model: model, Timeout: timeout, MaxBody: maxEmbedReply})
}

type vectorConfig struct {
	settings    recall.VectorSettings
	pool        string
	url         string
	model       string
	queryPrefix string
	timeout     time.Duration
	maxChars    int
	shadowPool  string
	shadow      recall.Pool
	shadowErr   error
	rerank      rerankConfig
	off         error
}

func (v vectorConfig) active() bool { return v.settings.Enabled() }

func (v vectorConfig) reranks() bool { return v.active() && v.rerank.on() }

func (v vectorConfig) shadowActive() bool { return v.shadow != recall.PoolNone }

func (v vectorConfig) embeds() bool { return v.active() || v.shadowActive() }

type embedResult struct {
	vec  []float32
	err  error
	took time.Duration
}

type pendingEmbed struct {
	done     chan struct{}
	res      embedResult
	start    time.Time
	deadline time.Time
}

func embedLimit(ctx context.Context, want time.Duration) time.Duration {
	dl, ok := ctx.Deadline()
	if !ok {
		return want
	}
	return min(want, time.Until(dl)*3/4)
}

func startEmbed(ctx context.Context, vc vectorConfig, query string) *pendingEmbed {
	start := time.Now()
	limit := embedLimit(ctx, vc.timeout)
	p := &pendingEmbed{done: make(chan struct{}), start: start, deadline: start.Add(limit + embedSlack)}
	newEmbedder := embedderFor
	go func() {
		var res embedResult
		defer func() {
			if r := recover(); r != nil {
				res = embedResult{err: fmt.Errorf("%w: %v", errEmbedPanic, r)}
			}
			res.took = time.Since(start)
			p.res = res
			close(p.done)
		}()
		text := query
		if vc.maxChars > 0 {
			text = clip(text, vc.maxChars)
		}
		ectx, cancel := context.WithTimeout(ctx, limit)
		defer cancel()
		vs, err := newEmbedder(vc.url, vc.model, limit).Embed(ectx, []string{vc.queryPrefix + text})
		switch {
		case err != nil:
			res.err = err
		case len(vs) != 1:
			res.err = fmt.Errorf("%w: %d vectors for one text", recall.ErrQuery, len(vs))
		default:
			res.vec, res.err = vs[0], recall.CheckQuery(vs[0])
		}
	}()
	return p
}

func (p *pendingEmbed) wait(ctx context.Context) (embedResult, bool) {
	timer := time.NewTimer(max(time.Until(p.deadline), 0))
	defer timer.Stop()
	select {
	case <-p.done:
		return p.res, true
	case <-timer.C:
	case <-ctx.Done():
	}
	return embedResult{}, false
}

func outcomeOf(err error) string {
	switch {
	case errors.Is(err, recall.ErrDimension):
		return outcomeDim
	case errors.Is(err, recall.ErrNoVectors):
		return outcomeNoVecs
	case errors.Is(err, errEmbedPanic):
		return outcomeError
	case errors.Is(err, recall.ErrQuery):
		return vec.FailResponse
	}
	return vec.Classify(err)
}

type chosen struct {
	recall.Packed
	labels        []string
	n             int
	pointers      []string
	pointerLabels []string
	cos           map[string]float64
	pointerCos    map[string]float64
	rerank        map[string]float64
}

func (c chosen) key(k string, cos float64) statefile.Key {
	out := statefile.Key{Key: k, Cos: turnCos(cos)}
	if s, ok := c.rerank[k]; ok {
		out.Rerank = rerankPtr(s)
	}
	return out
}

func (c chosen) rerankLabels() []string {
	var out []string
	for _, k := range append(slices.Clone(c.Keys), c.pointers...) {
		if s, ok := c.rerank[k]; ok {
			out = append(out, fmt.Sprintf("%s=%.4f", k, s))
		}
	}
	return out
}

func turnCos(c float64) float64 {
	if math.IsNaN(c) || math.IsInf(c, 0) {
		return 0
	}
	return roundCos(c)
}

func (c chosen) fillTurn(t *statefile.Turn) {
	if t == nil {
		return
	}
	for _, k := range c.Keys {
		if cos, ok := c.cos[k]; ok {
			t.Strong = append(t.Strong, c.key(k, cos))
		} else {
			t.Lexical = append(t.Lexical, k)
		}
	}
	for _, k := range c.pointers {
		t.Pointers = append(t.Pointers, c.key(k, c.pointerCos[k]))
	}
	t.Chars = utf8.RuneCountInString(c.Text)
}

type ranked struct {
	ix         *recall.Index
	scored     []recall.VHit
	expired    []recall.VHit
	words, tri []store.ListHit
	built      []recall.VHit
}

func (cfg config) searchQuery(query string, loc store.Here, exclude []string) store.Query {
	q := recall.SearchQuery(query, cfg.recall.Pool, nil, exclude)
	q.Here, q.Before = &loc, cfg.asOf
	return q
}

func rankVectors(ctx context.Context, s *store.Store, cfg config, vs recall.VectorSettings, query string, loc store.Here, exclude []string, qvec []float32) (ranked, string) {
	q := store.Query{Here: &loc, ExcludeKeys: exclude, Statuses: []store.Status{store.Current, store.Expired}, Before: cfg.asOf}
	if !vs.Pool.Logs() {
		q.ExcludeKinds = []store.Kind{store.Log}
	}
	ix, err := recall.LoadIndex(ctx, s, cfg.vec.model, q)
	if err != nil {
		if o := outcomeOf(err); o != vec.FailResponse {
			return ranked{}, o
		}
		return ranked{}, outcomeError
	}
	scored, err := ix.Score(qvec)
	if err != nil {
		return ranked{}, outcomeOf(err)
	}
	rk := ranked{ix: ix}
	rk.scored, rk.expired = recall.SplitExpired(scored)
	if vs.Pool.Fused() {
		if rk.words, rk.tri, err = s.Lists(ctx, cfg.searchQuery(query, loc, exclude)); err != nil {
			return ranked{}, outcomeError
		}
	}
	rk.built = vs.Build(ix, rk.scored, rk.words, rk.tri)
	return rk, ""
}

type tiers struct {
	strong   recall.Selection
	pointers func(skip func(recall.VHit) bool) recall.Selection
	on       bool
	rerank   map[string]float64
	records  map[string]store.Record
}

func cosinePointers(vs recall.VectorSettings, built, expired []recall.VHit, skip func(recall.VHit) bool) recall.Selection {
	sel := vs.PointerSelect(built, skip)
	sel.Reg = append(sel.Reg, vs.ExpiredPointers(expired, skip)...)
	return sel
}

func cosineTiers(vs recall.VectorSettings, built, expired []recall.VHit) tiers {
	return tiers{
		strong: vs.Gate(built),
		pointers: func(skip func(recall.VHit) bool) recall.Selection {
			return cosinePointers(vs, built, expired, skip)
		},
		on: vs.PointersConfigured(),
	}
}

func rerankTiers(vs recall.VectorSettings, g recall.RerankGate, rr *rerankRun) tiers {
	return tiers{
		strong:   vs.RerankStrong(g, rr.list),
		pointers: func(skip func(recall.VHit) bool) recall.Selection { return vs.RerankPointers(g, rr.list, skip) },
		on:       g.Pointer > 0 && vs.PointerTake > 0,
		rerank:   rr.scores,
		records:  rr.records,
	}
}

func (t tiers) get(ctx context.Context, s *store.Store, key string) (store.Record, error) {
	if rec, ok := t.records[key]; ok {
		return rec, nil
	}
	return s.Get(ctx, key)
}

func cosLabel(h recall.VHit) string {
	return fmt.Sprintf("%s=%.3f", h.Key, h.Cos)
}

func vectorInject(ctx context.Context, cfg config, e env.Env, s *store.Store, query string, loc store.Here, exclude []string, pointerSeen map[string]bool, emb *pendingEmbed, lexPicks []recall.Candidate, r *result) (chosen, bool) {
	record := func(outcome string) {
		if err := counter.Bump(filepath.Join(e.StateDir(), CountersDir), outcome); err != nil {
			r.note("counter: %v", err)
		}
	}
	fail := func(outcome string, took time.Duration) (chosen, bool) {
		record(outcome)
		r.note("embed=%s", outcome)
		r.embedded(outcome, took)
		return chosen{}, false
	}
	res, ok := emb.wait(ctx)
	if !ok {
		return fail(vec.FailTimeout, time.Since(emb.start))
	}
	if res.err != nil {
		return fail(outcomeOf(res.err), res.took)
	}
	vs := cfg.vec.settings
	rk, outcome := rankVectors(ctx, s, cfg, vs, query, loc, exclude, res.vec)
	if outcome != "" {
		return fail(outcome, res.took)
	}
	ix, built := rk.ix, rk.built
	tr := cosineTiers(vs, built, rk.expired)
	var rr *rerankRun
	if cfg.vec.reranks() {
		if rr = rerankPool(ctx, cfg, s, query, rk, pointerSeen); rr != nil && rr.outcome == outcomeOK {
			tr = rerankTiers(vs, cfg.vec.rerank.gate, rr)
		}
	}
	sel := tr.strong
	var recs []store.Record
	var labels []string
	cos := map[string]float64{}
	add := func(rec store.Record, label string, vh *recall.VHit) {
		if !slices.ContainsFunc(recs, func(x store.Record) bool { return x.Key == rec.Key }) {
			recs = append(recs, rec)
			labels = append(labels, label)
			if vh != nil {
				cos[rec.Key] = vh.Cos
			}
		}
	}
	if vs.LexRegular {
		for _, c := range lexPicks {
			add(c.Hit.Record, fmt.Sprintf("%s=lex", c.Hit.Record.Key), nil)
		}
	} else {
		for _, h := range sel.Reg {
			rec, err := tr.get(ctx, s, h.Key)
			if err != nil {
				return fail(outcomeError, res.took)
			}
			add(rec, cosLabel(h), &h)
		}
	}
	unembedded := 0
	if !vs.LexRegular {
		for _, c := range lexPicks {
			if !ix.Has(c.Hit.Record.Key) {
				add(c.Hit.Record, fmt.Sprintf("%s=lex-novec", c.Hit.Record.Key), nil)
				unembedded++
			}
		}
	}
	for _, h := range sel.Log {
		rec, err := tr.get(ctx, s, h.Key)
		if err != nil {
			return fail(outcomeError, res.took)
		}
		add(rec, cosLabel(h), &h)
	}
	record(outcomeOK)
	r.note("embed=ok %dms pool %s", res.took.Milliseconds(), cfg.vec.pool)
	r.embedded(outcomeOK, res.took)
	if rr != nil {
		record(rerankCounter + rr.outcome)
		r.note("rerank=%s %dms n %d", rr.outcome, rr.took.Milliseconds(), rr.n)
		rr.fillTurn(r.turn)
	}
	if unembedded > 0 {
		r.note("lexical gate for %d records without a vector", unembedded)
	}
	var strong recall.Packed
	if len(recs) > 0 {
		strong = recall.Pack(recall.PromptHeader, recs, query, cfg.recall.MaxChars)
	}
	ptrs, ptrLabels := pointerRecords(ctx, s, tr, recs, pointerSeen)
	out := chosen{Packed: strong, labels: labels, n: len(recs), cos: cos, pointerCos: map[string]float64{}, rerank: tr.rerank}
	for _, h := range built {
		out.pointerCos[h.Key] = h.Cos
	}
	for _, h := range rk.expired {
		out.pointerCos[h.Key] = h.Cos
	}
	if rr != nil {
		for _, h := range rr.list {
			out.pointerCos[h.Key] = h.Cos
		}
	}
	if len(ptrs) > 0 {
		room := cfg.recall.MaxChars - utf8.RuneCountInString(strong.Text)
		if strong.Text != "" {
			room--
		}
		packed := recall.PackPointers(ptrs, room)
		if len(packed.Dropped) > 0 {
			r.note("pointers over %d chars, left out %s", cfg.recall.MaxChars, strings.Join(packed.Dropped, ","))
		}
		if packed.Text != "" {
			out.Text = strings.Join(nonEmpty(strong.Text, packed.Text), "\n")
			out.pointers = packed.Keys
			out.pointerLabels = ptrLabels[:len(packed.Keys)]
			out.n += len(packed.Keys)
		}
	}
	return out, true
}

func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pointerRecords(ctx context.Context, s *store.Store, tr tiers, strong []store.Record, seen map[string]bool) ([]store.Record, []string) {
	if !tr.on {
		return nil, nil
	}
	sel := tr.pointers(pointerSkip(strong, seen))
	var recs []store.Record
	var labels []string
	for _, h := range append(append([]recall.VHit{}, sel.Reg...), sel.Log...) {
		rec, err := tr.get(ctx, s, h.Key)
		if err != nil {
			continue
		}
		recs = append(recs, rec)
		labels = append(labels, cosLabel(h))
	}
	return recs, labels
}

func pointerSkip(strong []store.Record, seen map[string]bool) func(recall.VHit) bool {
	return func(h recall.VHit) bool {
		return seen[h.Key] || slices.ContainsFunc(strong, func(x store.Record) bool { return x.Key == h.Key })
	}
}

func joinLabels(l []string) string { return strings.Join(l, ",") }

func addVectorFlags(fs *flag.FlagSet, cfg *config, e env.Env) {
	vc := &cfg.vec
	vc.settings = recall.DefaultVectorSettings()
	vs := &vc.settings
	prefix := e.Getenv("MEM_EMBED_QUERY_PREFIX")
	if prefix == "" {
		prefix = recall.DefaultQueryPrefix
	}
	fs.StringVar(&vc.pool, "vec-pool", "lexical", "candidate pool: lexical (no embedding), reg, fused-reg, all or fused-all")
	fs.StringVar(&vc.shadowPool, "shadow-vec-pool", "", "log what this pool would inject next to the live decision into $XDG_STATE_HOME/agent-memory/shadow.jsonl, never injected: reg, fused-reg, all or fused-all; empty for off")
	fs.BoolVar(&vs.LexRegular, "vec-lex-regular", false, "regular records come from the lexical gate, the vector pool supplies log records only")
	fs.StringVar(&vc.url, "embed-url", e.Getenv("MEM_EMBED_URL"), "embedding server base URL; POST /v1/embeddings (env MEM_EMBED_URL)")
	fs.StringVar(&vc.model, "embed-model", e.Getenv("MEM_EMBED_MODEL"), "model name; selects the stored vectors (env MEM_EMBED_MODEL)")
	fs.StringVar(&vc.queryPrefix, "embed-query-prefix", prefix, `text put before the query; \n, \t and \\ are expanded (env MEM_EMBED_QUERY_PREFIX)`)
	fs.DurationVar(&vc.timeout, "embed-timeout", 300*time.Millisecond, "hard limit of the query embedding; past it the lexical path answers")
	fs.IntVar(&vc.maxChars, "embed-max-chars", 0, "embed only the first N chars of the query, 0 for all")
	fs.IntVar(&vs.Reg.Take, "reg-take", vs.Reg.Take, "regular records taken from the vector pool, 0 for none")
	fs.Float64Var(&vs.Reg.MinCos, "reg-cos", vs.Reg.MinCos, "least cosine of a regular record")
	fs.Float64Var(&vs.Reg.MinMargin, "reg-margin", vs.Reg.MinMargin, "least gap between the first and the second regular hit, -3 for none")
	fs.Float64Var(&vs.Reg.PointerCos, "reg-pointer-cos", 0, "regular records with this cosine up to --reg-cos are listed as one-line pointers, 0 for none")
	fs.Float64Var(&vs.Log.PointerCos, "log-pointer-cos", 0, "log chunks with this cosine up to --log-cos are listed as one-line pointers, 0 for none")
	fs.IntVar(&vs.PointerTake, "pointer-take", vs.PointerTake, "pointers per kind, 0 for none")
	fs.IntVar(&vs.Log.Take, "log-take", vs.Log.Take, "log chunks taken from the vector pool, 0 for none")
	fs.Float64Var(&vs.Log.MinCos, "log-cos", 0.5, "least cosine of a log chunk")
	fs.Float64Var(&vs.Log.MinMargin, "log-margin", vs.Log.MinMargin, "least gap between the first and the second log hit, -3 for none")
	fs.Float64Var(&vs.Weights.Vec, "vec-weight", vs.Weights.Vec, "weight of the vector list in the fused pools")
	fs.Float64Var(&vs.Weights.Words, "rrf-words", vs.Weights.Words, "weight of the words list in the fused pools")
	fs.Float64Var(&vs.Weights.Tri, "rrf-tri", vs.Weights.Tri, "weight of the trigram list in the fused pools")
	fs.IntVar(&vs.Depth, "vec-depth", vs.Depth, "vector hits entering the fusion")
	addRerankFlags(fs, &vc.rerank, e)
}

func (v *vectorConfig) finish() error {
	pool, err := recall.ParsePool(v.pool)
	if err != nil {
		return err
	}
	v.settings.Pool = pool
	v.queryPrefix = vec.ExpandEscapes(v.queryPrefix)
	if v.active() {
		if err := v.checkEmbedding(); err != nil {
			return err
		}
	}
	shadow, err := recall.ParsePool(v.shadowPool)
	if err == nil && shadow != recall.PoolNone && !v.active() {
		err = v.checkEmbedding()
	}
	if err != nil {
		v.shadowErr = fmt.Errorf("--shadow-vec-pool: %w", err)
		return nil
	}
	v.shadow = shadow
	return nil
}

func (v vectorConfig) checkEmbedding() error {
	switch {
	case v.url == "":
		return errors.New("--vec-pool and --shadow-vec-pool need --embed-url")
	case v.model == "":
		return errors.New("--vec-pool and --shadow-vec-pool need --embed-model")
	case v.timeout <= 0:
		return errors.New("--embed-timeout must be positive")
	case v.settings.Reg.Take < 0 || v.settings.Log.Take < 0 || v.settings.Depth < 1:
		return errors.New("--reg-take and --log-take must not be negative, --vec-depth must be positive")
	case v.settings.PointerTake < 0 || v.settings.Reg.PointerCos < 0 || v.settings.Log.PointerCos < 0:
		return errors.New("--pointer-take, --reg-pointer-cos and --log-pointer-cos must not be negative")
	}
	return nil
}

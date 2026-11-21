package hook

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"time"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/statefile"
	"mem/internal/store"
	"mem/internal/vec"
)

const (
	maxRerankReply = 1 << 20
	rerankCounter  = "rerank-"
)

var errRerankPanic = errors.New("reranker panicked")

type Reranker interface {
	Rerank(ctx context.Context, query string, docs []string) ([]float64, error)
}

func defaultReranker(url string, timeout time.Duration) Reranker {
	return vec.NewReranker(vec.RerankOptions{URL: url, Timeout: timeout, MaxBody: maxRerankReply})
}

var rerankerFor = defaultReranker

type rerankConfig struct {
	url        string
	timeout    time.Duration
	candidates int
	docChars   int
	queryChars int
	gate       recall.RerankGate
	err        error
}

func (rc rerankConfig) on() bool { return rc.url != "" && rc.err == nil }

func addRerankFlags(fs *flag.FlagSet, rc *rerankConfig, e env.Env) {
	rc.gate = recall.DefaultRerankGate()
	fs.StringVar(&rc.url, "rerank-url", e.Getenv("MEM_RERANK_URL"), "cross-encoder base URL, POST /v1/rerank (llama.cpp --reranking); with a vector --vec-pool the rerank score gates both tiers instead of the cosine, any failure keeps the cosine gate (env MEM_RERANK_URL)")
	fs.DurationVar(&rc.timeout, "rerank-timeout", 1500*time.Millisecond, "limit of the rerank request, at most 3/4 of the budget left")
	fs.IntVar(&rc.candidates, "rerank-candidates", recall.DefaultRerankCandidates, "top records of the vector pool sent to the reranker")
	fs.IntVar(&rc.docChars, "rerank-doc-chars", recall.DefaultRerankDocChars, "chars of the body sent after the title, 0 for all")
	fs.IntVar(&rc.queryChars, "rerank-query-chars", recall.DefaultRerankQueryChars, "chars of the query sent, 0 for all")
	fs.Float64Var(&rc.gate.Text, "rerank-text", rc.gate.Text, "least rerank score of a record injected in full")
	fs.Float64Var(&rc.gate.Pointer, "rerank-pointer", rc.gate.Pointer, "least rerank score of a pointer line, 0 for none")
	fs.IntVar(&rc.gate.Take, "rerank-take", rc.gate.Take, "most records injected in full by the rerank gate")
}

func (rc *rerankConfig) finish() {
	if rc.url == "" {
		return
	}
	g := rc.gate
	switch {
	case rc.candidates < 1:
		rc.err = errors.New("--rerank-candidates must be positive")
	case rc.timeout <= 0:
		rc.err = errors.New("--rerank-timeout must be positive")
	case rc.docChars < 0 || rc.queryChars < 0 || g.Take < 0:
		rc.err = errors.New("--rerank-doc-chars, --rerank-query-chars and --rerank-take must not be negative")
	case !(g.Text >= 0 && g.Text <= 1) || !(g.Pointer >= 0 && g.Pointer <= g.Text):
		rc.err = errors.New("--rerank-text must lie in [0,1] and --rerank-pointer in [0,--rerank-text]")
	}
}

type rerankResult struct {
	scores []float64
	err    error
	took   time.Duration
}

func runRerank(ctx context.Context, rc rerankConfig, query string, docs []string) rerankResult {
	start := time.Now()
	limit := embedLimit(ctx, rc.timeout)
	if limit <= 0 {
		return rerankResult{err: context.DeadlineExceeded}
	}
	done := make(chan rerankResult, 1)
	newReranker := rerankerFor
	go func() {
		var res rerankResult
		defer func() {
			if p := recover(); p != nil {
				res = rerankResult{err: fmt.Errorf("%w: %v", errRerankPanic, p)}
			}
			done <- res
		}()
		rctx, cancel := context.WithTimeout(ctx, limit)
		defer cancel()
		scores, err := newReranker(rc.url, limit).Rerank(rctx, query, docs)
		if err == nil {
			err = vec.CheckScores(scores, len(docs))
		}
		res = rerankResult{scores: scores, err: err}
	}()
	timer := time.NewTimer(limit + embedSlack)
	defer timer.Stop()
	var res rerankResult
	select {
	case res = <-done:
	case <-timer.C:
		res = rerankResult{err: context.DeadlineExceeded}
	case <-ctx.Done():
		res = rerankResult{err: ctx.Err()}
	}
	res.took = time.Since(start)
	return res
}

func rerankOutcome(err error) string {
	switch {
	case err == nil:
		return outcomeOK
	case errors.Is(err, errRerankPanic):
		return outcomeError
	case errors.Is(err, vec.ErrBadRerank):
		return vec.FailResponse
	}
	return vec.Classify(err)
}

type rerankRun struct {
	outcome string
	took    time.Duration
	n       int
	list    []recall.Reranked
	scores  map[string]float64
	records map[string]store.Record
}

func (rr *rerankRun) fillTurn(t *statefile.Turn) {
	if rr == nil || t == nil {
		return
	}
	t.Rerank, t.RerankMs = rr.outcome, rr.took.Milliseconds()
	for _, h := range rr.list {
		t.Reranked = append(t.Reranked, statefile.Key{Key: h.Key, Cos: turnCos(h.Cos), Rerank: rerankPtr(h.Rerank)})
	}
}

func rerankPtr(s float64) *float64 {
	if math.IsNaN(s) || math.IsInf(s, 0) {
		return nil
	}
	v := roundCos(s)
	return &v
}

func rerankCandidates(ctx context.Context, cfg config, s *store.Store, rk ranked, pointerSeen map[string]bool) ([]recall.VHit, map[string]store.Record, error) {
	vs := cfg.vec.settings
	rc := cfg.vec.rerank
	pool := rk.built
	if rc.candidates > vs.PoolSize {
		wide := vs
		wide.PoolSize = rc.candidates
		pool = wide.Build(rk.ix, rk.scored, rk.words, rk.tri)
	}
	cands := vs.RerankCandidates(pool, rc.gate, rc.candidates)
	if rc.gate.Pointer > 0 && vs.PointerTake > 0 {
		cands = append(cands, vs.ExpiredNear(cands, rk.expired, recall.ExpiredCandidates, pointerSkip(nil, pointerSeen))...)
	}
	recs := make(map[string]store.Record, len(cands))
	for _, h := range cands {
		rec, err := s.Get(ctx, h.Key)
		if err != nil {
			return nil, nil, err
		}
		recs[h.Key] = rec
	}
	return cands, recs, nil
}

func rerankPool(ctx context.Context, cfg config, s *store.Store, query string, rk ranked, pointerSeen map[string]bool) *rerankRun {
	rc := cfg.vec.rerank
	start := time.Now()
	cands, recs, err := rerankCandidates(ctx, cfg, s, rk, pointerSeen)
	if err != nil {
		return &rerankRun{outcome: outcomeError, took: time.Since(start)}
	}
	if len(cands) == 0 {
		return nil
	}
	docs := make([]string, len(cands))
	for i, h := range cands {
		docs[i] = recall.RerankDoc(recs[h.Key], rc.docChars)
	}
	res := runRerank(ctx, rc, recall.ClipRunes(query, rc.queryChars), docs)
	run := &rerankRun{outcome: rerankOutcome(res.err), took: res.took, n: len(cands)}
	if res.err != nil {
		return run
	}
	run.records, run.scores = recs, make(map[string]float64, len(cands))
	for i, h := range cands {
		run.list = append(run.list, recall.Reranked{VHit: h, Rerank: res.scores[i]})
		run.scores[h.Key] = res.scores[i]
	}
	return run
}

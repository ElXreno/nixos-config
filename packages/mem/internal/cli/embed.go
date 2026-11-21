package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mem/internal/counter"
	"mem/internal/env"
	"mem/internal/hook"
	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

type embedFlags struct {
	url, model, docPrefix string
}

func (ef *embedFlags) bind(f *flags, e env.Env) {
	f.StringVar(&ef.url, "url", e.Getenv("MEM_EMBED_URL"), "embedding server base URL; POST /v1/embeddings (env MEM_EMBED_URL)")
	f.StringVar(&ef.model, "model", e.Getenv("MEM_EMBED_MODEL"), "model name, also the key of the stored vectors (env MEM_EMBED_MODEL)")
	f.StringVar(&ef.docPrefix, "doc-prefix", docPrefixDefault(e), `text put before every record, pass "" for none; \n, \t and \\ are expanded (env MEM_EMBED_DOC_PREFIX)`)
}

func docPrefixDefault(e env.Env) string {
	if p := e.Getenv("MEM_EMBED_DOC_PREFIX"); p != "" {
		return p
	}
	return recall.DefaultDocPrefix
}

func queryPrefixDefault(e env.Env) string {
	if p := e.Getenv("MEM_EMBED_QUERY_PREFIX"); p != "" {
		return p
	}
	return recall.DefaultQueryPrefix
}

func cmdReindex(e env.Env, args []string) int {
	f := newFlags(&e, "reindex", "--url URL --model M --doc-prefix S [--batch N] [--par N] [--skip-logs] [--force] [--max-docs N]", true)
	var ef embedFlags
	ef.bind(f, e)
	batch := f.Int("batch", recall.DefaultReindexBatch, "texts per request")
	par := f.Int("par", recall.DefaultReindexPar, "requests in flight")
	retries := f.Int("retries", vec.DefaultRetries, "retries of a request after a network error, 429 or 5xx")
	timeout := f.Duration("timeout", 10*time.Minute, "timeout of one request")
	skipLogs := f.Bool("skip-logs", false, "leave log records without vectors")
	force := f.Bool("force", false, "embed every current record again")
	maxDocs := f.Int("max-docs", 0, "embed at most N records this run, the shortest first, 0 for all")
	autoTopic := f.Bool("auto-topic", true, "give records without a topic the topic their nearest vectors vote for")
	autoK := f.Int("auto-topic-k", recall.DefaultAutoTopicK, "neighbours that vote on a topic")
	autoMin := f.Float64("auto-topic-min-cos", recall.DefaultAutoTopicMinCos, "cosine the nearest neighbour must reach before a topic is assigned")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	switch {
	case len(pos) > 0:
		_, _ = fmt.Fprintf(e.Stderr, "mem reindex: unexpected argument %q\n", pos[0])
		return 2
	case ef.url == "":
		_, _ = fmt.Fprintln(e.Stderr, "mem reindex: --url is required")
		return 2
	case ef.model == "":
		_, _ = fmt.Fprintln(e.Stderr, "mem reindex: --model is required")
		return 2
	case *batch < 1 || *par < 1 || *retries < 0 || *maxDocs < 0:
		_, _ = fmt.Fprintln(e.Stderr, "mem reindex: --batch and --par must be at least 1, --retries and --max-docs not negative")
		return 2
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	res, err := recall.Reindex(ctx, s, recall.ReindexOptions{
		Model: ef.model, DocPrefix: vec.ExpandEscapes(ef.docPrefix), Batch: *batch, Par: *par, SkipLogs: *skipLogs, Force: *force, MaxDocs: *maxDocs, Progress: e.Stderr,
		Embedder: vec.NewClient(vec.Options{URL: ef.url, Model: ef.model, Retries: *retries, Timeout: *timeout}),
	})
	if err != nil {
		_, _ = fmt.Fprintf(e.Stderr, "mem reindex: %v (embedded %d before it)\n", err, res.Embedded)
		return 1
	}
	if *autoTopic {
		at, err := recall.AutoTopics(ctx, s, recall.AutoTopicOptions{Model: ef.model, K: *autoK, MinCos: *autoMin})
		if err != nil {
			_, _ = fmt.Fprintf(e.Stderr, "mem reindex: auto topics: %v\n", err)
			return 1
		}
		for _, p := range at.Assigned {
			_, _ = fmt.Fprintf(e.Stderr, "topic %s -> %s (cos %.3f)\n", p.Key, p.Topic, p.Cos)
		}
	}
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, res))
	}
	_, _ = fmt.Fprintf(e.Stdout, "model %s, current %d, expired %d, fresh %d, embedded %d, raced %d, skipped logs %d, deferred %d, dim %d, %.1fs\n",
		ef.model, res.Current, res.Expired, res.Fresh, res.Embedded, res.Raced, res.SkippedLogs, res.Deferred, res.Dim, res.Seconds)
	return 0
}

type vecStatus struct {
	Records  int               `json:"records"`
	Current  int               `json:"current"`
	Expired  int               `json:"expired"`
	Models   []store.ModelRows `json:"models"`
	Model    string            `json:"model,omitempty"`
	Fresh    int               `json:"fresh"`
	Stale    int               `json:"stale"`
	Missing  int               `json:"missing"`
	Counters map[string]int64  `json:"counters"`
}

func cmdVec(e env.Env, args []string) int {
	if len(args) > 0 && args[0] == "pairs" {
		return cmdVecPairs(e, args[1:])
	}
	if len(args) == 0 || args[0] != "status" {
		_, _ = fmt.Fprintln(e.Stderr, "usage: mem vec status [--model M --doc-prefix S] [--json]\n       mem vec pairs [--model M] [--min-cos F] [--limit N] [--all] [--json]")
		return 2
	}
	f := newFlags(&e, "vec status", "[--model M --doc-prefix S] [--json]", true)
	var ef embedFlags
	ef.bind(f, e)
	pos, code, ok := f.parse(args[1:])
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem vec status: unexpected argument %q\n", pos[0])
		return 2
	}
	s, err := openStore(e, true)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	all, err := s.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return fail(e, err)
	}
	st := vecStatus{Records: len(all), Model: ef.model}
	if st.Models, err = s.VectorModels(ctx); err != nil {
		return fail(e, err)
	}
	if st.Counters, err = counter.Counts(filepath.Join(e.StateDir(), hook.CountersDir)); err != nil {
		return fail(e, err)
	}
	if ef.model != "" {
		info, err := s.VectorInfo(ctx, ef.model)
		if err != nil {
			return fail(e, err)
		}
		prefix := vec.ExpandEscapes(ef.docPrefix)
		for _, r := range all {
			switch r.Status {
			case store.Current:
				st.Current++
			case store.Expired:
				st.Expired++
			default:
				continue
			}
			switch vi, ok := info[r.Key]; {
			case !ok:
				st.Missing++
			case vi.SHA == store.TextSHA(store.DocText(prefix, r)):
				st.Fresh++
			default:
				st.Stale++
			}
		}
	} else {
		for _, r := range all {
			switch r.Status {
			case store.Current:
				st.Current++
			case store.Expired:
				st.Expired++
			}
		}
	}
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, st))
	}
	_, _ = fmt.Fprintf(e.Stdout, "records %d current %d expired %d\n", st.Records, st.Current, st.Expired)
	if len(st.Models) == 0 {
		_, _ = fmt.Fprintln(e.Stdout, "no vectors")
	}
	for _, m := range st.Models {
		_, _ = fmt.Fprintf(e.Stdout, "%s dim %d rows %d\n", m.Model, m.Dim, m.Rows)
	}
	if ef.model != "" {
		_, _ = fmt.Fprintf(e.Stdout, "model %s fresh %d stale %d missing %d\n", ef.model, st.Fresh, st.Stale, st.Missing)
	}
	names := make([]string, 0, len(st.Counters))
	for n := range st.Counters {
		names = append(names, n)
	}
	slices.Sort(names)
	var parts []string
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s %d", n, st.Counters[n]))
	}
	if len(parts) > 0 {
		_, _ = fmt.Fprintf(e.Stdout, "hook embed outcomes: %s\n", strings.Join(parts, ", "))
	}
	return 0
}

const minPairsCos = 0.5

func cmdVecPairs(e env.Env, args []string) int {
	f := newFlags(&e, "vec pairs", "[--model M] [--min-cos F] [--limit N] [--all] [--json]", true)
	model := f.String("model", e.Getenv("MEM_EMBED_MODEL"), "model name of the stored vectors (env MEM_EMBED_MODEL)")
	minCos := f.Float64("min-cos", recall.DefaultPairsMinCos, fmt.Sprintf("cosine a pair must reach, %g to 1", minPairsCos))
	limit := f.Int("limit", recall.DefaultPairsLimit, "pairs listed, 0 for all; the totals count every pair")
	all := f.Bool("all", false, "list the pairs the lexical duplicate gate flags too")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	switch {
	case len(pos) > 0:
		_, _ = fmt.Fprintf(e.Stderr, "mem vec pairs: unexpected argument %q\n", pos[0])
		return 2
	case *model == "":
		_, _ = fmt.Fprintln(e.Stderr, "mem vec pairs: --model is required")
		return 2
	case *limit < 0 || !(*minCos >= minPairsCos && *minCos <= 1):
		_, _ = fmt.Fprintf(e.Stderr, "mem vec pairs: --limit must not be negative, --min-cos must lie between %g and 1\n", minPairsCos)
		return 2
	}
	s, err := openStore(e, true)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	recs, err := s.List(ctx, store.Filter{})
	if err != nil {
		return fail(e, err)
	}
	vecs := map[string][]float32{}
	err = s.ScanVectors(ctx, *model, store.Query{ExcludeKinds: []store.Kind{store.Log}}, func(v store.VecScan) error {
		vecs[v.Key] = store.DecodeVec(v.Blob, nil)
		return nil
	})
	if err != nil {
		return fail(e, err)
	}
	if len(vecs) == 0 {
		return fail(e, fmt.Errorf("no vectors of model %s; run mem reindex", *model))
	}
	res := recall.Pairs(recs, vecs, recall.PairOptions{MinCos: *minCos, All: *all, Limit: *limit})
	res.Model = *model
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, res))
	}
	_, _ = fmt.Fprintf(e.Stdout, "pairs with cosine >= %.2f: %d, flagged by the lexical gate: %d, past the gate: %d\n", res.MinCos, res.Total, res.Gated, res.Past)
	for _, p := range res.Pairs {
		_, _ = fmt.Fprintf(e.Stdout, "%.3f  %s  %s  [%s/%s]  sim %.2f  shared %d\n", p.Cos, p.A, p.B, p.KindA, p.KindB, p.Sim, p.Shared)
	}
	return 0
}

type searchVector struct {
	url, model, prefix string
	timeout            time.Duration
	hybrid, noLogs     bool
}

func (sv searchVector) run(ctx context.Context, s *store.Store, q store.Query) ([]store.Hit, error) {
	vc := vec.NewClient(vec.Options{URL: sv.url, Model: sv.model, Timeout: sv.timeout})
	ectx, cancel := context.WithTimeout(ctx, sv.timeout)
	defer cancel()
	vs, err := vc.Embed(ectx, []string{sv.prefix + q.Text})
	if err != nil {
		return nil, fmt.Errorf("embedding the query (%s): %w", vec.Classify(err), err)
	}
	if len(vs) != 1 {
		return nil, errors.New("the embedder returned no vector")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = store.DefaultLimit
	}
	iq := store.Query{Scopes: q.Scopes, Kinds: q.Kinds, ExcludeKinds: q.ExcludeKinds, Statuses: q.Statuses, AllStatus: q.AllStatus}
	ix, err := recall.LoadIndex(ctx, s, sv.model, iq)
	if err != nil {
		return nil, err
	}
	scored, err := ix.Score(vs[0])
	if err != nil {
		return nil, err
	}
	settings := recall.DefaultVectorSettings()
	settings.PoolSize = limit
	settings.Pool = recall.PoolAll
	var words, tri []store.ListHit
	if sv.hybrid {
		settings.Pool = recall.PoolFusedAll
		lq := iq
		lq.Text, lq.Limit, lq.Depth, lq.Mode, lq.Fallback = q.Text, limit, max(recall.SearchDepth, limit), store.Fused, q.Fallback
		if words, tri, err = s.Lists(ctx, lq); err != nil {
			return nil, err
		}
	}
	var hits []store.Hit
	for _, h := range settings.Build(ix, scored, words, tri) {
		rec, err := s.Get(ctx, h.Key)
		if err != nil {
			return nil, err
		}
		hits = append(hits, store.Hit{Record: rec, Score: h.Cos, Snippet: q.Snippet(rec.Body, store.SnippetLen)})
	}
	return hits, nil
}

func searchVectorFlags(f *flags, e env.Env) (*searchVector, func() error) {
	sv := &searchVector{}
	f.StringVar(&sv.url, "embed-url", e.Getenv("MEM_EMBED_URL"), "embedding server base URL for --mode vector and hybrid (env MEM_EMBED_URL)")
	f.StringVar(&sv.model, "embed-model", e.Getenv("MEM_EMBED_MODEL"), "model name of the stored vectors (env MEM_EMBED_MODEL)")
	f.StringVar(&sv.prefix, "embed-query-prefix", queryPrefixDefault(e), "text put before the query")
	f.DurationVar(&sv.timeout, "embed-timeout", 10*time.Second, "limit of the query embedding")
	f.BoolVar(&sv.noLogs, "no-logs", false, "accepted for older calls: log records are left out unless --logs")
	return sv, func() error {
		sv.prefix = vec.ExpandEscapes(sv.prefix)
		if sv.url == "" || sv.model == "" {
			return errors.New("--mode vector and hybrid need --embed-url and --embed-model")
		}
		return nil
	}
}

type searchRerank struct {
	on, off, auto                bool
	url                          string
	candidates, docChars, qChars int
	timeout                      time.Duration
}

type rerankedHit struct {
	store.Hit
	Rerank *float64 `json:"rerank,omitempty"`
}

func searchRerankFlags(f *flags, e env.Env) (*searchRerank, func() error) {
	sr := &searchRerank{}
	f.BoolVar(&sr.on, "rerank", false, "rerank the top --rerank-candidates hits with a cross-encoder and order them by its score, the hits past them follow in fused order; on by itself when no --mode is given and both services are configured")
	f.BoolVar(&sr.off, "no-rerank", false, "never rerank")
	f.StringVar(&sr.url, "rerank-url", e.Getenv("MEM_RERANK_URL"), "cross-encoder base URL, POST /v1/rerank (env MEM_RERANK_URL)")
	f.IntVar(&sr.candidates, "rerank-candidates", recall.DefaultRerankCandidates, "hits sent to the reranker")
	f.IntVar(&sr.docChars, "rerank-doc-chars", recall.DefaultRerankDocChars, "chars of the body sent after the title, 0 for all")
	f.IntVar(&sr.qChars, "rerank-query-chars", recall.DefaultRerankQueryChars, "chars of the query sent, 0 for all")
	f.DurationVar(&sr.timeout, "rerank-timeout", 10*time.Second, "limit of the rerank request")
	return sr, func() error {
		switch {
		case sr.on && sr.off:
			return errors.New("--rerank and --no-rerank exclude each other")
		case !sr.on:
			return nil
		case sr.url == "":
			return errors.New("--rerank needs --rerank-url")
		case sr.candidates < 1 || sr.timeout <= 0:
			return errors.New("--rerank-candidates and --rerank-timeout must be positive")
		case sr.docChars < 0 || sr.qChars < 0:
			return errors.New("--rerank-doc-chars and --rerank-query-chars must not be negative")
		}
		return nil
	}
}

func (sr searchRerank) run(ctx context.Context, query string, hits []store.Hit, limit int) ([]rerankedHit, error) {
	cands := hits[:min(sr.candidates, len(hits))]
	docs := make([]string, len(cands))
	for i, h := range cands {
		docs[i] = recall.RerankDoc(h.Record, sr.docChars)
	}
	rctx, cancel := context.WithTimeout(ctx, sr.timeout)
	defer cancel()
	scores, err := vec.NewReranker(vec.RerankOptions{URL: sr.url, Timeout: sr.timeout}).Rerank(rctx, recall.ClipRunes(query, sr.qChars), docs)
	if err != nil {
		return nil, fmt.Errorf("rerank (%s): %w", vec.Classify(err), err)
	}
	out := make([]rerankedHit, len(cands))
	for i, h := range cands {
		out[i] = rerankedHit{Hit: h, Rerank: &scores[i]}
	}
	slices.SortStableFunc(out, func(a, b rerankedHit) int {
		switch {
		case *a.Rerank > *b.Rerank:
			return -1
		case *a.Rerank < *b.Rerank:
			return 1
		}
		return 0
	})
	for _, h := range hits[len(cands):] {
		out = append(out, rerankedHit{Hit: h})
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

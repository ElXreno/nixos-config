package ui

import (
	"context"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

const (
	maxRemoteCalls  = 2
	embedTimeout    = 4 * time.Second
	rerankTimeout   = 10 * time.Second
	rerankDocChars  = 3000
	rerankDefaultN  = recall.DefaultRerankCandidates
	rerankMaxN      = 30
	modeWords       = "words"
	modeTrigram     = "trigram"
	modeFused       = "fused"
	modeVector      = "vector"
	modeHybrid      = "hybrid"
	errBusy         = "busy"
	errNotConfigure = "not-configured"
)

type hitOut struct {
	Key       string   `json:"key"`
	Hit       string   `json:"hit"`
	Kind      string   `json:"kind"`
	Status    string   `json:"status"`
	Title     string   `json:"title"`
	Score     float64  `json:"score"`
	WordsRank int      `json:"words_rank,omitempty"`
	TriRank   int      `json:"tri_rank,omitempty"`
	VecRank   int      `json:"vec_rank,omitempty"`
	WordsBM25 float64  `json:"words_bm25,omitempty"`
	TriBM25   float64  `json:"tri_bm25,omitempty"`
	Overlap   float64  `json:"overlap,omitempty"`
	Cos       *float64 `json:"cos,omitempty"`
	Rerank    *float64 `json:"rerank,omitempty"`
	Snippet   string   `json:"snippet"`
}

type searchOut struct {
	Query    string            `json:"query"`
	Mode     string            `json:"mode"`
	Asked    string            `json:"asked"`
	Reranked bool              `json:"reranked"`
	Degraded map[string]string `json:"degraded,omitempty"`
	EmbedMs  int64             `json:"embed_ms,omitempty"`
	RerankMs int64             `json:"rerank_ms,omitempty"`
	TotalMs  int64             `json:"total_ms"`
	Hits     []hitOut          `json:"hits"`
}

type searchReq struct {
	text    string
	limit   int
	mode    string
	rerank  bool
	rerankN int
	query   store.Query
}

func (s *Server) parseSearch(r *http.Request) (searchReq, error) {
	q := r.URL.Query()
	req := searchReq{text: strings.TrimSpace(q.Get("q")), mode: q.Get("mode"), rerank: q.Get("rerank") == "1"}
	if req.text == "" || len(req.text) > maxQueryLen {
		return req, errors.New("q must be 1-" + strconv.Itoa(maxQueryLen) + " bytes")
	}
	limit, err := intParam(q, "limit", defaultFound, maxFound)
	if err != nil || limit == 0 {
		return req, errors.New("limit must be 1-" + strconv.Itoa(maxFound))
	}
	req.limit = limit
	req.rerankN, err = intParam(q, "rn", rerankDefaultN, rerankMaxN)
	if err != nil || req.rerankN == 0 {
		return req, errors.New("rn must be 1-" + strconv.Itoa(rerankMaxN))
	}
	switch req.mode {
	case "":
		req.mode = modeFused
	case modeWords, modeTrigram, modeFused, modeVector, modeHybrid:
	default:
		return req, store.ErrInvalid
	}
	query := store.Query{Text: req.text, Limit: limit, Fallback: true}
	switch st := q.Get("status"); st {
	case "current":
	case "", "all":
		query.AllStatus = true
	default:
		parsed, err := store.ParseStatus(st)
		if err != nil {
			return req, err
		}
		query.Statuses = []store.Status{parsed}
	}
	for _, v := range q["kind"] {
		for _, part := range strings.Split(v, ",") {
			k, err := store.ParseKind(part)
			if err != nil {
				return req, err
			}
			query.Kinds = append(query.Kinds, k)
		}
	}
	for _, v := range q["project"] {
		for _, part := range strings.Split(v, ",") {
			sc, err := store.NormalizeScope(store.ProjectScope(part))
			if err != nil {
				return req, err
			}
			query.Scopes = append(query.Scopes, sc)
		}
	}
	if q.Get("logs") == "0" {
		query.ExcludeKinds = []store.Kind{store.Log}
	}
	switch req.mode {
	case modeWords:
		query.Mode = store.WordsOnly
	case modeTrigram:
		query.Mode = store.TrigramOnly
	}
	req.query = query
	return req, nil
}

func (s *Server) acquire() bool {
	select {
	case s.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) release() { <-s.gate }

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	req, err := s.parseSearch(r)
	if err != nil {
		if errors.Is(err, store.ErrInvalid) {
			s.fail(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	out := searchOut{Query: req.text, Mode: req.mode, Asked: req.mode, Hits: []hitOut{}}
	remote := req.mode == modeVector || req.mode == modeHybrid || req.rerank
	if remote {
		if !s.acquire() {
			writeError(w, http.StatusTooManyRequests, errBusy)
			return
		}
		defer s.release()
	}
	var hits []hitOut
	if req.mode == modeVector || req.mode == modeHybrid {
		var ok bool
		hits, ok = s.vectorSearch(ctx, req, &out)
		if !ok {
			out.Mode = modeFused
			req.query.Mode = store.Fused
			hits, err = s.lexicalSearch(ctx, req.query)
			if err != nil {
				s.fail(w, err)
				return
			}
		}
	} else {
		hits, err = s.lexicalSearch(ctx, req.query)
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	if req.rerank {
		hits = s.rerankHits(ctx, req, hits, &out)
	}
	if len(hits) > req.limit {
		hits = hits[:req.limit]
	}
	out.Hits = hits
	out.TotalMs = time.Since(start).Milliseconds()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) degrade(out *searchOut, part, why string) {
	if out.Degraded == nil {
		out.Degraded = map[string]string{}
	}
	out.Degraded[part] = why
}

func outOf(h store.Hit) hitOut {
	key := h.Record.Key
	if h.Record.Kind == store.Log {
		key, _ = BaseKey(key)
	}
	return hitOut{
		Key: key, Hit: h.Record.Key, Kind: string(h.Record.Kind), Status: string(h.Record.Status), Title: h.Record.Title,
		Score: h.Score, WordsRank: h.WordsRank, TriRank: h.TriRank, WordsBM25: h.WordsBM25, TriBM25: h.TriBM25, Overlap: h.Overlap, Snippet: h.Snippet,
	}
}

func (s *Server) lexicalSearch(ctx context.Context, q store.Query) ([]hitOut, error) {
	hits, err := s.st.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]hitOut, 0, len(hits))
	for _, h := range hits {
		out = append(out, outOf(h))
	}
	return out, nil
}

func (s *Server) httpOr() *http.Client {
	return s.opt.HTTP
}

func (s *Server) modelName(ctx context.Context) string {
	if s.opt.EmbedModel != "" {
		return s.opt.EmbedModel
	}
	if g, err := s.Graph(ctx); err == nil {
		return g.vec.Model
	}
	return ""
}

func (s *Server) vectorSearch(ctx context.Context, req searchReq, out *searchOut) ([]hitOut, bool) {
	model := s.modelName(ctx)
	if s.opt.EmbedURL == "" || model == "" {
		s.degrade(out, "embed", errNotConfigure)
		return nil, false
	}
	g, err := s.Graph(ctx)
	if err != nil {
		s.degrade(out, "embed", "store")
		return nil, false
	}
	t0 := time.Now()
	ectx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()
	qv, err := vec.NewClient(vec.Options{URL: s.opt.EmbedURL, Model: model, Timeout: embedTimeout, Client: s.httpOr()}).Embed(ectx, []string{s.opt.queryPrefix() + req.text})
	out.EmbedMs = time.Since(t0).Milliseconds()
	if err != nil || len(qv) != 1 {
		s.degrade(out, "embed", vec.Classify(err))
		return nil, false
	}
	entries, err := s.vectorEntries(ctx, model, g.fp)
	if err != nil {
		s.degrade(out, "embed", "no-vectors")
		return nil, false
	}
	allowed := s.entryFilter(g, req)
	sub := make([]recall.Entry, 0, len(entries))
	for _, e := range entries {
		if allowed(e.Key) {
			sub = append(sub, e)
		}
	}
	if len(sub) == 0 {
		return []hitOut{}, true
	}
	ix := recall.NewIndex(sub)
	scored, err := ix.Score(qv[0])
	if err != nil {
		s.degrade(out, "embed", "dimension")
		return nil, false
	}
	settings := recall.DefaultVectorSettings()
	settings.PoolSize = max(req.limit, req.rerankN)
	settings.Pool = recall.PoolAll
	var words, tri []store.ListHit
	var fusedScore map[string]float64
	if req.mode == modeHybrid {
		settings.Pool = recall.PoolFusedAll
		lq := req.query
		lq.Mode, lq.Depth, lq.Limit = store.Fused, max(recall.SearchDepth, settings.PoolSize), settings.PoolSize
		if words, tri, err = s.st.Lists(ctx, lq); err != nil {
			return nil, false
		}
		fusedScore = fusedScores(scored, words, tri, settings)
	}
	wr, tr := map[string]store.ListHit{}, map[string]store.ListHit{}
	for _, h := range words {
		wr[h.Key] = h
	}
	for _, h := range tri {
		tr[h.Key] = h
	}
	vecRank := map[string]int{}
	for i, h := range scored {
		vecRank[h.Key] = i + 1
	}
	var hits []hitOut
	for _, h := range settings.Build(ix, scored, words, tri) {
		rec, err := s.st.Get(ctx, h.Key)
		if err != nil {
			continue
		}
		ho := outOf(store.Hit{Record: rec, Snippet: store.Snippet(rec.Body, req.text, store.SnippetLen)})
		cos := h.Cos
		ho.Cos = &cos
		ho.VecRank = vecRank[h.Key]
		if req.mode == modeVector {
			ho.Score = cos
		} else if sc, ok := fusedScore[h.Key]; ok {
			ho.Score = sc
		}
		if w, ok := wr[h.Key]; ok {
			ho.WordsRank, ho.WordsBM25 = w.Rank, w.Score
		}
		if t, ok := tr[h.Key]; ok {
			ho.TriRank, ho.TriBM25 = t.Rank, t.Score
		}
		hits = append(hits, ho)
	}
	if hits == nil {
		hits = []hitOut{}
	}
	return hits, true
}

func fusedScores(scored []recall.VHit, words, tri []store.ListHit, v recall.VectorSettings) map[string]float64 {
	seen := map[string]bool{}
	var vkeys []string
	for _, h := range scored {
		if seen[h.File] {
			continue
		}
		seen[h.File] = true
		vkeys = append(vkeys, h.Key)
		if len(vkeys) == v.Depth {
			break
		}
	}
	out := map[string]float64{}
	for _, f := range recall.Fuse(words, tri, vkeys, v.Weights, math.MaxInt) {
		out[f.Key] = f.Score
	}
	return out
}

func (s *Server) entryFilter(g *Graph, req searchReq) func(string) bool {
	q := req.query
	return func(key string) bool {
		base, _ := BaseKey(key)
		m := g.meta[base]
		if m == nil {
			m = g.meta[key]
		}
		if m == nil {
			return false
		}
		if !q.AllStatus {
			if len(q.Statuses) > 0 {
				if !slices.Contains(q.Statuses, m.status) {
					return false
				}
			} else if m.status != store.Current {
				return false
			}
		}
		if len(q.Kinds) > 0 && !slices.Contains(q.Kinds, m.kind) {
			return false
		}
		if slices.Contains(q.ExcludeKinds, m.kind) {
			return false
		}
		if len(q.Scopes) > 0 && !slices.ContainsFunc(q.Scopes, func(sc string) bool { return slices.Contains(m.scopes, sc) }) {
			return false
		}
		return true
	}
}

func (s *Server) rerankHits(ctx context.Context, req searchReq, hits []hitOut, out *searchOut) []hitOut {
	if s.opt.RerankURL == "" {
		s.degrade(out, "rerank", errNotConfigure)
		return hits
	}
	n := min(req.rerankN, len(hits))
	if n == 0 {
		return hits
	}
	docs := make([]string, n)
	for i := range n {
		rec, err := s.st.Get(ctx, hits[i].Hit)
		if err != nil {
			s.degrade(out, "rerank", "store")
			return hits
		}
		docs[i] = recall.RerankDoc(rec, rerankDocChars)
	}
	t0 := time.Now()
	rctx, cancel := context.WithTimeout(ctx, rerankTimeout)
	defer cancel()
	scores, err := vec.NewReranker(vec.RerankOptions{URL: s.opt.RerankURL, Timeout: rerankTimeout, Client: s.httpOr()}).
		Rerank(rctx, recall.ClipRunes(req.text, recall.DefaultRerankQueryChars), docs)
	out.RerankMs = time.Since(t0).Milliseconds()
	if err != nil {
		s.degrade(out, "rerank", vec.Classify(err))
		return hits
	}
	head := slices.Clone(hits[:n])
	for i := range head {
		v := scores[i]
		head[i].Rerank = &v
	}
	slices.SortStableFunc(head, func(a, b hitOut) int {
		switch {
		case *a.Rerank > *b.Rerank:
			return -1
		case *a.Rerank < *b.Rerank:
			return 1
		}
		return 0
	})
	out.Reranked = true
	return append(head, hits[n:]...)
}

type vecCache struct {
	fp      string
	model   string
	entries []recall.Entry
}

func (s *Server) vectorEntries(ctx context.Context, model, fp string) ([]recall.Entry, error) {
	s.vecMu.Lock()
	defer s.vecMu.Unlock()
	if c := s.vecIdx; c != nil && c.fp == fp && c.model == model {
		return c.entries, nil
	}
	ix, err := recall.LoadIndex(ctx, s.st, model, store.Query{AllStatus: true})
	if err != nil {
		return nil, err
	}
	s.vecIdx = &vecCache{fp: fp, model: model, entries: ix.Entries}
	return ix.Entries, nil
}

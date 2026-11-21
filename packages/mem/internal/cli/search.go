package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

const (
	autoEmbedTimeout  = 3 * time.Second
	autoRerankTimeout = 6 * time.Second
)

func vectorOutcome(err error) string {
	if errors.Is(err, recall.ErrNoVectors) {
		return "has no stored vectors for this model"
	}
	if errors.Is(err, recall.ErrDimension) {
		return "returns another dimension than the stored vectors"
	}
	return vec.Classify(err)
}

func cmdSearch(e env.Env, args []string) int {
	f := newFlags(&e, "search", "QUERY... [flags]", true)
	limit := f.Int("limit", store.DefaultLimit, "maximum hits")
	all := f.Bool("all-status", false, "include superseded and retired records; expired state records are always listed, marked EXPIRED")
	var kinds, scopes multi
	f.Var(&kinds, "kind", "only these kinds; repeatable, comma-separated")
	f.Var(&scopes, "scope", "only records with one of these scopes; repeatable, comma-separated")
	mode := f.String("mode", "", "fused, words, trigram, vector or hybrid; without it: hybrid plus rerank when an embedder and a reranker are configured, fused when they are not, do not answer or --all-chunks is given")
	chunks := f.Bool("all-chunks", false, "list every matching chunk of a log file, not only the best one; implies --logs; lexical only: without --mode it selects fused, with --mode vector or hybrid it is a usage error")
	logs := f.Bool("logs", false, "include log records; they are left out otherwise")
	sv, svCheck := searchVectorFlags(f, e)
	sr, srCheck := searchRerankFlags(f, e)
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if *limit <= 0 {
		*limit = store.DefaultLimit
	}
	q := store.Query{Text: strings.Join(pos, " "), Limit: *limit, AllStatus: *all, Scopes: scopes, AllChunks: *chunks, Fallback: true}
	if strings.TrimSpace(q.Text) == "" {
		_, _ = fmt.Fprintln(e.Stderr, "mem search: needs a query")
		return 2
	}
	if !*all {
		q.Statuses = []store.Status{store.Current, store.Expired}
	}
	var err error
	if q.Kinds, err = parseKinds(kinds); err != nil {
		return fail(e, err)
	}
	if !*logs && !*chunks && !slices.Contains(q.Kinds, store.Log) {
		q.ExcludeKinds = []store.Kind{store.Log}
	}
	vectorMode, auto := false, false
	switch *mode {
	case "":
		switch {
		case *chunks:
		case sv.url == "":
		case sv.model == "":
			_, _ = fmt.Fprintln(e.Stderr, "mem search: an embedder URL without a model (MEM_EMBED_MODEL), lexical results only")
		default:
			vectorMode, auto, sv.hybrid = true, true, true
			sv.prefix = vec.ExpandEscapes(sv.prefix)
			if !f.set("embed-timeout") {
				sv.timeout = autoEmbedTimeout
			}
			if sr.url != "" && !sr.on && !sr.off {
				sr.on, sr.auto = true, true
				if !f.set("rerank-timeout") {
					sr.timeout = autoRerankTimeout
				}
			}
		}
	case "vector", "hybrid":
		vectorMode, sv.hybrid = true, *mode == "hybrid"
		if *chunks {
			_, _ = fmt.Fprintf(e.Stderr, "mem search: --all-chunks works only in the lexical modes fused, words or trigram, not with --mode %s\n", *mode)
			return 2
		}
		if err := svCheck(); err != nil {
			_, _ = fmt.Fprintln(e.Stderr, "mem search:", err)
			return 2
		}
	case "fused":
	case "words":
		q.Mode = store.WordsOnly
	case "trigram":
		q.Mode = store.TrigramOnly
	default:
		_, _ = fmt.Fprintf(e.Stderr, "mem search: unknown mode %q\n", *mode)
		return 2
	}
	if err := srCheck(); err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "mem search:", err)
		return 2
	}
	if sr.on {
		q.Limit = max(*limit, sr.candidates)
	}
	s, err := openStore(e, true)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	var hits []store.Hit
	if vectorMode {
		hits, err = sv.run(ctx, s, q)
		switch {
		case err != nil && auto:
			_, _ = fmt.Fprintf(e.Stderr, "mem search: embedder %s, lexical results only\n", vectorOutcome(err))
			vectorMode = false
			if sr.auto {
				sr.on = false
			}
		case err != nil:
			_, _ = fmt.Fprintln(e.Stderr, "mem search:", err)
			return 1
		}
	}
	if !vectorMode {
		if hits, err = s.Search(ctx, q); err != nil {
			return fail(e, err)
		}
	}
	out := []rerankedHit{}
	for _, h := range hits {
		out = append(out, rerankedHit{Hit: h})
	}
	if sr.on && len(hits) > 0 {
		ranked, err := sr.run(ctx, q.Text, hits, *limit)
		switch {
		case err != nil && sr.auto:
			_, _ = fmt.Fprintf(e.Stderr, "mem search: reranker %s, order left as fused\n", vec.Classify(errors.Unwrap(err)))
		case err != nil:
			_, _ = fmt.Fprintln(e.Stderr, "mem search:", err)
			return 1
		default:
			out = ranked
		}
	}
	if *limit > 0 && len(out) > *limit {
		out = out[:*limit]
	}
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, out))
	}
	if len(out) == 0 {
		_, _ = fmt.Fprintln(e.Stdout, "no matches")
		return 0
	}
	for i, h := range out {
		r := h.Record
		score := fmt.Sprintf("score %.4f", h.Score)
		if h.Rerank != nil {
			score = fmt.Sprintf("rerank %.4f %s", *h.Rerank, score)
		}
		state := fmt.Sprintf("%s, as of %s", r.Status, r.AsOf().Local().Format(time.DateOnly))
		if r.Status == store.Expired {
			state = "EXPIRED since " + r.ExpiredSince().Local().Format(time.DateOnly)
		}
		_, _ = fmt.Fprintf(e.Stdout, "%d. %s [%s, %s] %s\n", i+1, r.Key, r.Kind, state, score)
		if r.Title != "" {
			_, _ = fmt.Fprintf(e.Stdout, "   %s\n", r.Title)
		}
		_, _ = fmt.Fprintf(e.Stdout, "   %s\n", h.Snippet)
		if r.Status == store.Expired {
			_, _ = fmt.Fprintf(e.Stdout, "   expired state: check it, then `mem verify %s` if it still holds, `mem retire %s --why TEXT` if not\n", r.Key, r.Key)
		}
	}
	return 0
}

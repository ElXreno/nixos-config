package ui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"mem/internal/store"
)

const (
	maxQueryLen  = 500
	defaultPage  = 100
	maxPage      = 1000
	defaultFound = 20
	maxFound     = 100
)

type recordOut struct {
	store.Record
	Body   *string  `json:"body,omitempty"`
	Chunks []string `json:"chunks,omitempty"`
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such record")
	default:
		writeError(w, http.StatusInternalServerError, "store error")
	}
}

func intParam(q url.Values, name string, def, hi int) (int, error) {
	v := q.Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, errors.New(name + " must be a non-negative integer")
	}
	return min(n, hi), nil
}

func keyParam(r *http.Request) (string, error) {
	key := r.URL.Query().Get("key")
	if err := store.ValidKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) countBy(ctx context.Context, query string) (map[string]int, error) {
	rows, err := s.st.DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	snap, err := s.revision(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap.out)
}

func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{AllStatus: true}
	for _, v := range q["kind"] {
		for _, part := range strings.Split(v, ",") {
			k, err := store.ParseKind(part)
			if err != nil {
				s.fail(w, err)
				return
			}
			f.Kinds = append(f.Kinds, k)
		}
	}
	if st := q.Get("status"); st != "" && st != "all" {
		parsed, err := store.ParseStatus(st)
		if err != nil {
			s.fail(w, err)
			return
		}
		f.AllStatus, f.Statuses = false, []store.Status{parsed}
	}
	for _, v := range q["scope"] {
		sc, err := store.NormalizeScope(v)
		if err != nil {
			s.fail(w, err)
			return
		}
		f.Scopes = append(f.Scopes, sc)
	}
	limit, err := intParam(q, "limit", defaultPage, maxPage)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := intParam(q, "offset", 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recs, err := s.st.List(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	if topic := q.Get("topic"); topic != "" {
		want := strings.ToLower(topic)
		if want == "none" {
			want = NoTopic
		}
		recs = slices.DeleteFunc(recs, func(rec store.Record) bool { return rec.Topic() != want })
	}
	total := len(recs)
	if offset > total {
		offset = total
	}
	recs = recs[offset:min(offset+limit, total)]
	withBody := q.Get("body") == "1"
	out := make([]recordOut, len(recs))
	for i, rec := range recs {
		ro := recordOut{Record: rec}
		if withBody {
			b := rec.Body
			ro.Body = &b
		}
		out[i] = ro
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "offset": offset, "limit": limit, "records": out})
}

func (s *Server) chunkKeys(ctx context.Context, base string) ([]string, error) {
	rows, err := s.st.DB().QueryContext(ctx, "SELECT key FROM records WHERE kind = 'log' AND key GLOB ? ORDER BY key", base+"#[0-9]*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type ck struct {
		key string
		n   int
	}
	var all []ck
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		if b, n := BaseKey(k); b == base && n > 0 {
			all = append(all, ck{k, n})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n < all[j].n })
	out := make([]string, len(all))
	for i, c := range all {
		out[i] = c.key
	}
	return out, nil
}

func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	key, err := keyParam(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	ctx := r.Context()
	rec, err := s.st.Get(ctx, key)
	var chunks []string
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNotFound):
		if chunks, err = s.chunkKeys(ctx, key); err != nil {
			s.fail(w, err)
			return
		}
		if len(chunks) == 0 {
			s.fail(w, store.ErrNotFound)
			return
		}
		var bodies []string
		for i, ck := range chunks {
			c, err := s.st.Get(ctx, ck)
			if err != nil {
				s.fail(w, err)
				return
			}
			if i == 0 {
				rec = c
			}
			bodies = append(bodies, c.Body)
		}
		rec.Key, rec.Body = key, strings.Join(bodies, "\n\n")
	default:
		s.fail(w, err)
		return
	}
	body := rec.Body
	ro := recordOut{Record: rec, Body: &body, Chunks: chunks}
	links, err := s.linksOf(ctx, key)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"record": ro, "links": links})
}

func (s *Server) linksOf(ctx context.Context, key string) ([]store.Link, error) {
	keys := []string{key}
	chunks, err := s.chunkKeys(ctx, key)
	if err != nil {
		return nil, err
	}
	keys = append(keys, chunks...)
	out := []store.Link{}
	seen := map[store.Link]bool{}
	for _, k := range keys {
		ls, err := s.st.Links(ctx, k)
		if err != nil {
			return nil, err
		}
		for _, l := range ls {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out, nil
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	key, err := keyParam(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	links, err := s.linksOf(r.Context(), key)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "links": links})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	key, err := keyParam(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	ctx := r.Context()
	keys := []string{key}
	chunks, err := s.chunkKeys(ctx, key)
	if err != nil {
		s.fail(w, err)
		return
	}
	keys = append(keys, chunks...)
	versions := []store.Version{}
	for _, k := range keys {
		vs, err := s.st.History(ctx, k)
		if err != nil {
			s.fail(w, err)
			return
		}
		versions = append(versions, vs...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "versions": versions})
}

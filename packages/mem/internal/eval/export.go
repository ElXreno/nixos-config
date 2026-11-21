package eval

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mem/internal/env"
	"mem/internal/store"
)

const (
	SetCase     = "case"
	CaseSession = "case"
	QueriesFile = "queries.jsonl"
	DocsFile    = "docs.jsonl"
)

type QueryText struct {
	ID      string `json:"id"`
	Session string `json:"session"`
	Set     string `json:"set"`
	Text    string `json:"text"`
}

type DocText struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type ExportCounts struct {
	Gold     int `json:"gold"`
	Negative int `json:"negative"`
	Case     int `json:"case"`
	Docs     int `json:"docs"`
}

func (p *prepared) queryTexts() ([]QueryText, error) {
	var out []QueryText
	seen := map[string]bool{}
	for _, set := range []struct {
		name string
		qs   []*query
	}{{SetGold, p.qs}, {SetNegative, p.negs}, {SetCase, p.cases}} {
		for _, q := range set.qs {
			id := p.id(q)
			if seen[id] {
				return nil, fmt.Errorf("duplicate query id %s", id)
			}
			seen[id] = true
			out = append(out, QueryText{ID: id, Session: q.Session, Set: set.name, Text: q.Query})
		}
	}
	return out, nil
}

func (p *prepared) docTexts(ctx context.Context) ([]DocText, error) {
	recs, err := p.s.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return nil, err
	}
	out := make([]DocText, len(recs))
	for i, r := range recs {
		out[i] = DocText{Key: r.Key, Text: r.Title + "\n\n" + r.Body}
	}
	slices.SortFunc(out, func(a, b DocText) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}

func encodeJSONL[T any](w io.Writer, rows []T) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func sumJSONL[T any](rows []T) (string, error) {
	h := sha256.New()
	if err := encodeJSONL(h, rows); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeJSONL[T any](path string, rows []T) error {
	if err := writeFile(path, func(w *bufio.Writer) error { return encodeJSONL(w, rows) }); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func ExportTexts(ctx context.Context, e env.Env, cfg Config) (ExportCounts, error) {
	var c ExportCounts
	p, err := prepare(ctx, e, cfg)
	if err != nil {
		return c, err
	}
	defer func() { _ = p.s.Close() }()
	if err := os.MkdirAll(cfg.ExportTexts, 0o700); err != nil {
		return c, err
	}
	qs, err := p.queryTexts()
	if err != nil {
		return c, err
	}
	docs, err := p.docTexts(ctx)
	if err != nil {
		return c, err
	}
	if err := writeJSONL(filepath.Join(cfg.ExportTexts, QueriesFile), qs); err != nil {
		return c, err
	}
	if err := writeJSONL(filepath.Join(cfg.ExportTexts, DocsFile), docs); err != nil {
		return c, err
	}
	c = ExportCounts{Gold: len(p.qs), Negative: len(p.negs), Case: len(p.cases), Docs: len(docs)}
	return c, nil
}

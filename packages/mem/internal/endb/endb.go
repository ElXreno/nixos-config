package endb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mem/internal/store"
)

type Options struct {
	In           string
	EN           string
	Out          string
	AllowMissing bool
	Force        bool
}

type Result struct {
	Records    int `json:"records"`
	Replaced   int `json:"replaced"`
	Missing    int `json:"missing"`
	WithIssues int `json:"with_issues"`
}

type line struct {
	ID      int             `json:"id"`
	Key     string          `json:"key"`
	Title   string          `json:"title_en"`
	Body    string          `json:"body_en"`
	Issues  json.RawMessage `json:"issues"`
	lineNum int
}

func hasIssues(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "[]" && s != `""`
}

func readLines(path string) ([]line, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []line
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<28)
	n := 0
	for sc.Scan() {
		n++
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		l.lineNum = n
		out = append(out, l)
	}
	return out, sc.Err()
}

func removeDB(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func Build(ctx context.Context, o Options) (Result, error) {
	var res Result
	if o.In == "" || o.EN == "" || o.Out == "" {
		return res, errors.New("--db, --en and --out are required")
	}
	inAbs, err := filepath.Abs(o.In)
	if err != nil {
		return res, err
	}
	outAbs, err := filepath.Abs(o.Out)
	if err != nil {
		return res, err
	}
	if inAbs == outAbs {
		return res, errors.New("the output is the same file as the input database")
	}
	if _, err := os.Stat(outAbs); err == nil && !o.Force {
		return res, fmt.Errorf("%s exists; --force replaces it", outAbs)
	}
	lines, err := readLines(o.EN)
	if err != nil {
		return res, err
	}
	src, err := store.Open(ctx, inAbs, store.Options{ReadOnly: true})
	if err != nil {
		return res, err
	}
	defer func() { _ = src.Close() }()
	recs, err := src.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return res, err
	}
	res.Records = len(recs)
	known := make(map[string]bool, len(recs))
	titled := make(map[string]bool, len(recs))
	for _, r := range recs {
		known[r.Key] = true
		titled[r.Key] = strings.TrimSpace(r.Title) != ""
	}
	seen := map[string]bool{}
	texts := make([]store.Text, 0, len(lines))
	for _, l := range lines {
		switch {
		case !known[l.Key]:
			return res, fmt.Errorf("%s:%d: key %s is not in the database", o.EN, l.lineNum, l.Key)
		case seen[l.Key]:
			return res, fmt.Errorf("%s:%d: duplicate key %s", o.EN, l.lineNum, l.Key)
		case strings.TrimSpace(l.Body) == "":
			return res, fmt.Errorf("%s:%d: empty body_en for %s", o.EN, l.lineNum, l.Key)
		case titled[l.Key] && strings.TrimSpace(l.Title) == "":
			return res, fmt.Errorf("%s:%d: empty title_en for %s, the original has a title", o.EN, l.lineNum, l.Key)
		}
		seen[l.Key] = true
		if hasIssues(l.Issues) {
			res.WithIssues++
		}
		texts = append(texts, store.Text{Key: l.Key, Title: l.Title, Body: l.Body})
	}
	res.Missing = len(recs) - len(texts)
	if res.Missing > 0 && !o.AllowMissing {
		return res, fmt.Errorf("%d of %d records have no English text (missing); --allow-missing keeps their original text", res.Missing, len(recs))
	}
	if err := os.MkdirAll(filepath.Dir(outAbs), 0o700); err != nil {
		return res, err
	}
	if err := removeDB(outAbs); err != nil {
		return res, err
	}
	if _, err := src.DB().ExecContext(ctx, "VACUUM INTO ?", outAbs); err != nil {
		_ = removeDB(outAbs)
		return res, err
	}
	dst, err := store.Open(ctx, outAbs, store.Options{})
	if err != nil {
		_ = removeDB(outAbs)
		return res, err
	}
	n, err := dst.Retext(ctx, texts)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = removeDB(outAbs)
		return res, err
	}
	res.Replaced = n
	return res, os.Chmod(outAbs, 0o600)
}

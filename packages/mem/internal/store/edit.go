package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type MatchError struct {
	Key   string
	Count int
}

func (e *MatchError) Error() string {
	if e.Count == 0 {
		return fmt.Sprintf("the --old text does not occur in %s; `mem get --body %s` prints the body", e.Key, e.Key)
	}
	return fmt.Sprintf("the --old text occurs %d times in %s; add surrounding text until it is unique, or pass --all", e.Count, e.Key)
}

type EditRequest struct {
	Key string
	Old string
	New string
	All bool
	Why string
}

func (s *Store) Edit(ctx context.Context, req EditRequest) (PutResult, error) {
	if err := ValidKey(req.Key); err != nil {
		return PutResult{}, err
	}
	if req.Old == "" {
		return PutResult{}, invalid("edit needs a non-empty --old")
	}
	if req.Old == req.New {
		return PutResult{}, invalid("--old and --new are the same text")
	}
	why := strings.TrimSpace(req.Why)
	if why == "" {
		why = "edit"
	}
	var res PutResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.getRow(ctx, tx, req.Key)
		if err != nil {
			return err
		}
		n := countOverlapping(old.rec.Body, req.Old)
		if n == 0 || n > 1 && !req.All {
			return &MatchError{Key: req.Key, Count: n}
		}
		body := strings.ReplaceAll(old.rec.Body, req.Old, req.New)
		res, err = s.update(ctx, tx, PutRequest{Key: req.Key, Update: true, Body: &body, Why: why}, DefaultGate())
		return err
	})
	return res, err
}

func countOverlapping(s, sub string) int {
	n := 0
	for pos := 0; pos < len(s); {
		i := strings.Index(s[pos:], sub)
		if i < 0 {
			break
		}
		n++
		pos += i + 1
	}
	return n
}

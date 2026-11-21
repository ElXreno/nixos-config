package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Retimed struct {
	Key  string    `json:"key"`
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func triggerOnlyOrigin(cur Record, hist []Version, since time.Time) (time.Time, bool) {
	to, found := cur.UpdatedAt, false
	for _, prev := range slices.Backward(hist) {
		if prev.Version != cur.Version-1 || prev.Status != Current || prev.ChangedAt.Before(since) ||
			!strings.HasPrefix(prev.Reason, "update") || !sameClaim(prev.Record, cur) {
			break
		}
		to, found = prev.UpdatedAt, true
		cur = prev.Record
	}
	return to, found
}

func (s *Store) Retime(ctx context.Context, since time.Time, apply bool) ([]Retimed, error) {
	recs, err := s.List(ctx, Filter{Statuses: []Status{Current}})
	if err != nil {
		return nil, err
	}
	var out []Retimed
	for _, r := range recs {
		hist, err := s.History(ctx, r.Key)
		if err != nil {
			return nil, err
		}
		if to, ok := triggerOnlyOrigin(r, hist, since); ok && to.Before(r.UpdatedAt) {
			out = append(out, Retimed{Key: r.Key, From: r.UpdatedAt, To: to})
		}
	}
	if !apply || len(out) == 0 {
		return out, nil
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		for _, rt := range out {
			rw, err := s.getRow(ctx, tx, rt.Key)
			if err != nil {
				return err
			}
			if !rw.rec.UpdatedAt.Equal(rt.From) {
				return fmt.Errorf("record %s changed while its date was being restored", rt.Key)
			}
			rw.rec.UpdatedAt = rt.To
			if err := updateRecord(ctx, tx, rw.id, rw.rec); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

package store

import (
	"context"
	"database/sql"
	"strings"
)

type Text struct {
	Key   string
	Title string
	Body  string
}

func (s *Store) Retext(ctx context.Context, texts []Text) (int, error) {
	n := 0
	err := s.tx(ctx, func(tx *sql.Tx) error {
		n = 0
		for _, t := range texts {
			rw, err := s.getRow(ctx, tx, t.Key)
			if err != nil {
				return err
			}
			rw.rec.Title = strings.TrimSpace(t.Title)
			rw.rec.Body = cleanBody(t.Body)
			if rw.rec.Body == "" {
				return invalid("record %s has an empty body", t.Key)
			}
			if err := updateRecord(ctx, tx, rw.id, rw.rec); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

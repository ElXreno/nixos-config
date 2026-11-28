package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
)

const schemaV3 = `
CREATE TABLE vectors (
	key TEXT NOT NULL REFERENCES records (key) ON DELETE CASCADE ON UPDATE CASCADE,
	model TEXT NOT NULL,
	dim INTEGER NOT NULL CHECK (dim > 0),
	vec BLOB NOT NULL CHECK (length(vec) = dim * 4),
	text_sha256 TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (key, model)
) WITHOUT ROWID;
CREATE INDEX vectors_model ON vectors (model);
CREATE TRIGGER vectors_text_changed AFTER UPDATE OF title, body ON records
WHEN old.title <> new.title OR old.body <> new.body BEGIN
	DELETE FROM vectors WHERE key = new.key;
END;
`

func DocText(prefix string, r Record) string {
	return prefix + r.Title + "\n\n" + r.Body
}

func TextSHA(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type VecRow struct {
	Key     string
	Vec     []float32
	TextSHA string
}

func encodeVec(v []float32) []byte {
	raw := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(x))
	}
	return raw
}

func DecodeVec(raw []byte, dst []float32) []float32 {
	dst = slices.Grow(dst[:0], len(raw)/4)[:len(raw)/4]
	for i := range dst {
		dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}
	return dst
}

func (s *Store) PutVectors(ctx context.Context, model, docPrefix string, rows []VecRow) (written, skipped int, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		written, skipped = 0, 0
		now := fmtTime(s.clock())
		for _, r := range rows {
			if len(r.Vec) == 0 {
				return invalid("vector of %s is empty", r.Key)
			}
			rw, err := s.getRow(ctx, tx, r.Key)
			if err != nil {
				return err
			}
			if TextSHA(DocText(docPrefix, rw.rec)) != r.TextSHA {
				skipped++
				continue
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO vectors (key, model, dim, vec, text_sha256, updated_at) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (key, model) DO UPDATE SET dim = excluded.dim, vec = excluded.vec, text_sha256 = excluded.text_sha256, updated_at = excluded.updated_at`,
				r.Key, model, len(r.Vec), encodeVec(r.Vec), r.TextSHA, now)
			if err != nil {
				return fmt.Errorf("vector of %s: %w", r.Key, err)
			}
			written++
		}
		return nil
	})
	return written, skipped, err
}

type VecInfo struct {
	SHA string
	Dim int
}

func (s *Store) VectorInfo(ctx context.Context, model string) (map[string]VecInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, dim, text_sha256 FROM vectors WHERE model = ?`, model)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]VecInfo{}
	for rows.Next() {
		var k string
		var vi VecInfo
		if err := rows.Scan(&k, &vi.Dim, &vi.SHA); err != nil {
			return nil, err
		}
		out[k] = vi
	}
	return out, rows.Err()
}

type ModelRows struct {
	Model string `json:"model"`
	Dim   int    `json:"dim"`
	Rows  int    `json:"rows"`
}

func (s *Store) VectorModels(ctx context.Context) ([]ModelRows, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT model, dim, count(*) FROM vectors GROUP BY model, dim ORDER BY model, dim`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ModelRows
	for rows.Next() {
		var m ModelRows
		if err := rows.Scan(&m.Model, &m.Dim, &m.Rows); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type VecScan struct {
	Key    string
	Kind   Kind
	Status Status
	Source string
	Dim    int
	Blob   []byte
}

func (s *Store) ScanVectors(ctx context.Context, model string, q Query, fn func(VecScan) error) error {
	f := q.filter()
	f.see(s.access)
	args := append([]any{model}, f.args...)
	rows, err := s.db.QueryContext(ctx, `SELECT v.key, r.kind, r.status, r.source, v.dim, v.vec FROM vectors v JOIN records r ON r.key = v.key
		WHERE v.model = ?`+f.sql()+` ORDER BY v.key`, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v VecScan
		if err := rows.Scan(&v.Key, &v.Kind, &v.Status, &v.Source, &v.Dim, &v.Blob); err != nil {
			return err
		}
		if err := fn(v); err != nil {
			return err
		}
	}
	return rows.Err()
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrSchemaNewer    = errors.New("database schema is newer than this mem binary")
	ErrSchemaOutdated = errors.New("database schema needs a migration, open it writable once")
)

type Options struct {
	ReadOnly    bool
	BusyTimeout time.Duration
	Now         func() time.Time
}

type Store struct {
	db   *sql.DB
	path string
	now  func() time.Time
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func Open(ctx context.Context, path string, opt Options) (*Store, error) {
	if path == "" {
		return nil, errors.New("empty database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if opt.ReadOnly {
		if _, err := os.Stat(abs); err != nil {
			return nil, err
		}
	} else if err := createPrivate(abs); err != nil {
		return nil, err
	}
	busy := opt.BusyTimeout
	if busy <= 0 {
		busy = 5 * time.Second
	}
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busy.Milliseconds()))
	q.Add("_pragma", "foreign_keys(1)")
	if opt.ReadOnly {
		q.Set("mode", "ro")
	} else {
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(NORMAL)")
		q.Set("_txlock", "immediate")
	}
	dsn := "file:" + (&url.URL{Path: abs}).EscapedPath() + "?" + q.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	s := &Store{db: db, path: abs, now: now}
	if err := s.migrate(ctx, opt.ReadOnly); err != nil {
		_ = db.Close()
		return nil, err
	}
	if !opt.ReadOnly {
		if err := chmodPrivate(abs); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return s, nil
}

func createPrivate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func chmodPrivate(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		fi, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if fi.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(p, fi.Mode().Perm()&0o700); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) clock() time.Time { return s.now().UTC().Truncate(time.Second) }

func (s *Store) migrate(ctx context.Context, readOnly bool) error {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("%w: schema %d, binary knows %d", ErrSchemaNewer, v, len(migrations))
	}
	if v == len(migrations) {
		return nil
	}
	if readOnly {
		return fmt.Errorf("%w: schema %d, binary wants %d", ErrSchemaOutdated, v, len(migrations))
	}
	for i := v; i < len(migrations); i++ {
		err := s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
				return fmt.Errorf("migration %d: %w", i+1, err)
			}
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1))
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

var migrations = []string{schemaV1, schemaV2, schemaV3, schemaV4}

const schemaV2 = `ALTER TABLE links ADD COLUMN note TEXT NOT NULL DEFAULT '';`

var schemaV1 = `
CREATE TABLE records (
	id INTEGER PRIMARY KEY,
	key TEXT NOT NULL UNIQUE,
	kind TEXT NOT NULL CHECK (kind IN ('rule', 'fact', 'state', 'incident', 'howto', 'log')),
	core INTEGER NOT NULL DEFAULT 0 CHECK (core IN (0, 1)),
	title TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('current', 'superseded', 'expired', 'retired')),
	superseded_by TEXT NOT NULL DEFAULT '',
	valid_from TEXT NOT NULL,
	valid_to TEXT NOT NULL DEFAULT '',
	ttl_days INTEGER NOT NULL DEFAULT 0,
	verify_cmd TEXT NOT NULL DEFAULT '',
	verified_at TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	version INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX records_status_kind ON records (status, kind);

CREATE TABLE scopes (
	record_id INTEGER NOT NULL REFERENCES records (id) ON DELETE CASCADE,
	scope TEXT NOT NULL,
	PRIMARY KEY (record_id, scope)
) WITHOUT ROWID;
CREATE INDEX scopes_scope ON scopes (scope);

CREATE TABLE triggers (
	record_id INTEGER NOT NULL REFERENCES records (id) ON DELETE CASCADE,
	tool TEXT NOT NULL,
	regex TEXT NOT NULL,
	PRIMARY KEY (record_id, tool, regex)
) WITHOUT ROWID;

CREATE TABLE history (
	id INTEGER PRIMARY KEY,
	key TEXT NOT NULL,
	version INTEGER NOT NULL,
	kind TEXT NOT NULL,
	scopes TEXT NOT NULL,
	core INTEGER NOT NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL,
	status TEXT NOT NULL,
	superseded_by TEXT NOT NULL,
	valid_from TEXT NOT NULL,
	valid_to TEXT NOT NULL,
	ttl_days INTEGER NOT NULL,
	verify_cmd TEXT NOT NULL,
	verified_at TEXT NOT NULL,
	triggers TEXT NOT NULL,
	source TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	changed_at TEXT NOT NULL,
	reason TEXT NOT NULL
);
CREATE INDEX history_key ON history (key, version);

CREATE TABLE links (
	src TEXT NOT NULL,
	dst TEXT NOT NULL,
	rel TEXT NOT NULL CHECK (rel IN ('supersedes', 'relates', 'derived_from', 'unresolved')),
	PRIMARY KEY (src, dst, rel)
) WITHOUT ROWID;
CREATE INDEX links_dst ON links (dst);

CREATE VIRTUAL TABLE fts_words USING fts5 (
	key, title, body,
	content = '', contentless_delete = 1,
	tokenize = 'unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE fts_tri USING fts5 (
	key, title, body,
	content = '', contentless_delete = 1,
	tokenize = 'trigram remove_diacritics 1'
);

CREATE TRIGGER records_ai AFTER INSERT ON records BEGIN
	INSERT INTO fts_words (rowid, key, title, body) VALUES (new.id, new.key, ` + yo("new.title") + `, ` + yo("new.body") + `);
	INSERT INTO fts_tri (rowid, key, title, body) VALUES (new.id, new.key, ` + yo("new.title") + `, ` + yo("new.body") + `);
END;
CREATE TRIGGER records_ad AFTER DELETE ON records BEGIN
	DELETE FROM fts_words WHERE rowid = old.id;
	DELETE FROM fts_tri WHERE rowid = old.id;
END;
CREATE TRIGGER records_au AFTER UPDATE OF key, title, body ON records BEGIN
	DELETE FROM fts_words WHERE rowid = old.id;
	DELETE FROM fts_tri WHERE rowid = old.id;
	INSERT INTO fts_words (rowid, key, title, body) VALUES (new.id, new.key, ` + yo("new.title") + `, ` + yo("new.body") + `);
	INSERT INTO fts_tri (rowid, key, title, body) VALUES (new.id, new.key, ` + yo("new.title") + `, ` + yo("new.body") + `);
END;
`

func yo(col string) string {
	return "replace(replace(" + col + ", 'ё', 'е'), 'Ё', 'Е')"
}

const timeLayout = time.RFC3339

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(timeLayout, s)
}

const recordColumns = `r.id, r.key, r.kind, r.core, r.title, r.body, r.status, r.superseded_by, r.valid_from, r.valid_to,
	r.ttl_days, r.verify_cmd, r.verified_at, r.source, r.version, r.created_at, r.updated_at`

type row struct {
	id  int64
	rec Record
}

func scanRecord(sc interface{ Scan(...any) error }) (row, error) {
	var (
		rw                                                   row
		core                                                 int
		validFrom, validTo, verifiedAt, createdAt, updatedAt string
	)
	r := &rw.rec
	err := sc.Scan(&rw.id, &r.Key, &r.Kind, &core, &r.Title, &r.Body, &r.Status, &r.SupersededBy, &validFrom, &validTo,
		&r.TTLDays, &r.VerifyCmd, &verifiedAt, &r.Source, &r.Version, &createdAt, &updatedAt)
	if err != nil {
		return rw, err
	}
	r.Core = core == 1
	for _, f := range []struct {
		dst *time.Time
		src string
	}{{&r.ValidFrom, validFrom}, {&r.ValidTo, validTo}, {&r.VerifiedAt, verifiedAt}, {&r.CreatedAt, createdAt}, {&r.UpdatedAt, updatedAt}} {
		if *f.dst, err = parseTime(f.src); err != nil {
			return rw, fmt.Errorf("record %s: %w", r.Key, err)
		}
	}
	return rw, nil
}

func (s *Store) loadWhere(ctx context.Context, q querier, where string, args ...any) ([]row, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+recordColumns+" FROM records r "+where, args...)
	if err != nil {
		return nil, err
	}
	var out []row
	for rows.Next() {
		rw, err := scanRecord(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, rw)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attach(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) attach(ctx context.Context, q querier, rows []row) error {
	if len(rows) == 0 {
		return nil
	}
	idx := make(map[int64]int, len(rows))
	ids := make([]any, 0, len(rows))
	for i, r := range rows {
		idx[r.id] = i
		ids = append(ids, r.id)
	}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		in := placeholders(len(chunk))
		sr, err := q.QueryContext(ctx, "SELECT record_id, scope FROM scopes WHERE record_id IN ("+in+") ORDER BY record_id, scope", chunk...)
		if err != nil {
			return err
		}
		for sr.Next() {
			var id int64
			var scope string
			if err := sr.Scan(&id, &scope); err != nil {
				_ = sr.Close()
				return err
			}
			r := &rows[idx[id]].rec
			r.Scopes = append(r.Scopes, scope)
		}
		if err := sr.Close(); err != nil {
			return err
		}
		tr, err := q.QueryContext(ctx, "SELECT record_id, tool, regex FROM triggers WHERE record_id IN ("+in+") ORDER BY record_id, tool, regex", chunk...)
		if err != nil {
			return err
		}
		for tr.Next() {
			var id int64
			var t Trigger
			if err := tr.Scan(&id, &t.Tool, &t.Regex); err != nil {
				_ = tr.Close()
				return err
			}
			r := &rows[idx[id]].rec
			r.Triggers = append(r.Triggers, t)
		}
		if err := tr.Close(); err != nil {
			return err
		}
	}
	return nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

func (s *Store) getRow(ctx context.Context, q querier, key string) (row, error) {
	rows, err := s.loadWhere(ctx, q, "WHERE r.key = ?", key)
	if err != nil {
		return row{}, err
	}
	if len(rows) == 0 {
		return row{}, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return rows[0], nil
}

func (s *Store) Get(ctx context.Context, key string) (Record, error) {
	r, err := s.getRow(ctx, s.db, key)
	return r.rec, err
}

func insertRecord(ctx context.Context, q querier, r Record) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO records (key, kind, core, title, body, status, superseded_by, valid_from,
		valid_to, ttl_days, verify_cmd, verified_at, source, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Key, r.Kind, boolInt(r.Core), r.Title, r.Body, r.Status, r.SupersededBy, fmtTime(r.ValidFrom), fmtTime(r.ValidTo),
		r.TTLDays, r.VerifyCmd, fmtTime(r.VerifiedAt), r.Source, r.Version, fmtTime(r.CreatedAt), fmtTime(r.UpdatedAt))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, writeChildren(ctx, q, id, r)
}

func updateRecord(ctx context.Context, q querier, id int64, r Record) error {
	_, err := q.ExecContext(ctx, `UPDATE records SET kind = ?, core = ?, title = ?, body = ?, status = ?, superseded_by = ?,
		valid_from = ?, valid_to = ?, ttl_days = ?, verify_cmd = ?, verified_at = ?, source = ?, version = ?,
		created_at = ?, updated_at = ? WHERE id = ?`,
		r.Kind, boolInt(r.Core), r.Title, r.Body, r.Status, r.SupersededBy, fmtTime(r.ValidFrom), fmtTime(r.ValidTo),
		r.TTLDays, r.VerifyCmd, fmtTime(r.VerifiedAt), r.Source, r.Version, fmtTime(r.CreatedAt), fmtTime(r.UpdatedAt), id)
	if err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, "DELETE FROM scopes WHERE record_id = ?", id); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, "DELETE FROM triggers WHERE record_id = ?", id); err != nil {
		return err
	}
	return writeChildren(ctx, q, id, r)
}

func writeChildren(ctx context.Context, q querier, id int64, r Record) error {
	for _, sc := range r.Scopes {
		if _, err := q.ExecContext(ctx, "INSERT INTO scopes (record_id, scope) VALUES (?, ?)", id, sc); err != nil {
			return err
		}
	}
	for _, t := range r.Triggers {
		if _, err := q.ExecContext(ctx, "INSERT INTO triggers (record_id, tool, regex) VALUES (?, ?, ?)", id, t.Tool, t.Regex); err != nil {
			return err
		}
	}
	return nil
}

func insertHistory(ctx context.Context, q querier, r Record, changed time.Time, reason string) error {
	scopes, err := json.Marshal(nonNil(r.Scopes))
	if err != nil {
		return err
	}
	triggers, err := json.Marshal(nonNil(r.Triggers))
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO history (key, version, kind, scopes, core, title, body, status, superseded_by,
		valid_from, valid_to, ttl_days, verify_cmd, verified_at, triggers, source, created_at, updated_at, changed_at, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Key, r.Version, r.Kind, string(scopes), boolInt(r.Core), r.Title, r.Body, r.Status, r.SupersededBy,
		fmtTime(r.ValidFrom), fmtTime(r.ValidTo), r.TTLDays, r.VerifyCmd, fmtTime(r.VerifiedAt), string(triggers), r.Source,
		fmtTime(r.CreatedAt), fmtTime(r.UpdatedAt), fmtTime(changed), reason)
	return err
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func addLink(ctx context.Context, q querier, l Link) error {
	if _, err := ParseRel(string(l.Rel)); err != nil {
		return err
	}
	if l.Src == "" || l.Dst == "" {
		return invalid("link needs src and dst")
	}
	_, err := q.ExecContext(ctx, `INSERT INTO links (src, dst, rel, note) VALUES (?, ?, ?, ?)
		ON CONFLICT (src, dst, rel) DO UPDATE SET note = excluded.note WHERE excluded.note != ''`, l.Src, l.Dst, l.Rel, l.Note)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) History(ctx context.Context, key string) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, version, kind, scopes, core, title, body, status, superseded_by,
		valid_from, valid_to, ttl_days, verify_cmd, verified_at, triggers, source, created_at, updated_at, changed_at, reason
		FROM history WHERE key = ? ORDER BY version, id`, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Version
	for rows.Next() {
		var (
			v                                                             Version
			scopes, triggers                                              string
			core                                                          int
			validFrom, validTo, verifiedAt, createdAt, updatedAt, changed string
		)
		r := &v.Record
		if err := rows.Scan(&r.Key, &r.Version, &r.Kind, &scopes, &core, &r.Title, &r.Body, &r.Status, &r.SupersededBy,
			&validFrom, &validTo, &r.TTLDays, &r.VerifyCmd, &verifiedAt, &triggers, &r.Source, &createdAt, &updatedAt,
			&changed, &v.Reason); err != nil {
			return nil, err
		}
		r.Core = core == 1
		if err := json.Unmarshal([]byte(scopes), &r.Scopes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(triggers), &r.Triggers); err != nil {
			return nil, err
		}
		if len(r.Triggers) == 0 {
			r.Triggers = nil
		}
		for _, f := range []struct {
			dst *time.Time
			src string
		}{{&r.ValidFrom, validFrom}, {&r.ValidTo, validTo}, {&r.VerifiedAt, verifiedAt}, {&r.CreatedAt, createdAt}, {&r.UpdatedAt, updatedAt}, {&v.ChangedAt, changed}} {
			if *f.dst, err = parseTime(f.src); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Links(ctx context.Context, key string) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT src, dst, rel, note FROM links WHERE src = ? OR dst = ? ORDER BY src, rel, dst", key, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.Src, &l.Dst, &l.Rel, &l.Note); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) AddLinks(ctx context.Context, links []Link) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, l := range links {
			if err := addLink(ctx, tx, l); err != nil {
				return err
			}
		}
		return nil
	})
}

func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func (s *Store) Optimize(ctx context.Context) error {
	for _, stmt := range []string{
		"INSERT INTO fts_words (fts_words) VALUES ('optimize')",
		"INSERT INTO fts_tri (fts_tri) VALUES ('optimize')",
		"PRAGMA optimize",
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

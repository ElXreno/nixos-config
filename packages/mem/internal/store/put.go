package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"mem/internal/text"
)

type Gate struct {
	Dup        float64
	Rel        float64
	MinShared  int
	Neighbours int
	Skip       bool
}

const (
	DefaultDup        = 0.45
	DefaultRel        = 0.25
	RelFloor          = 0.1
	RelTopic          = 2
	DefaultMinShared  = 2
	DefaultNeighbours = 5
)

func DefaultGate() Gate {
	return Gate{Dup: DefaultDup, Rel: DefaultRel, MinShared: DefaultMinShared, Neighbours: DefaultNeighbours}
}

func (g Gate) withDefaults() Gate {
	d := DefaultGate()
	if g.Dup <= 0 {
		g.Dup = d.Dup
	}
	if g.Rel <= 0 {
		g.Rel = d.Rel
	}
	if g.MinShared <= 0 {
		g.MinShared = d.MinShared
	}
	if g.Neighbours <= 0 {
		g.Neighbours = d.Neighbours
	}
	return g
}

func (g Gate) Duplicate(c Candidate) bool {
	return c.Similarity >= g.Dup && c.Shared >= g.MinShared
}

type PutRequest struct {
	Key        string
	Kind       Kind
	Scopes     []string
	Core       *bool
	Title      *string
	Body       *string
	Triggers   *[]Trigger
	VerifyCmd  *string
	TTLDays    *int
	Source     *string
	Update     bool
	Supersedes []string
	Distinct   []string
	Why        string
	Gate       Gate
	AsOf       time.Time
	Links      []Link
}

type Candidate struct {
	Record     Record  `json:"record"`
	Similarity float64 `json:"similarity"`
	Shared     int     `json:"shared"`
	Topic      int     `json:"topic"`
	Snippet    string  `json:"snippet"`
}

type PutResult struct {
	Key        string      `json:"key"`
	Version    int         `json:"version"`
	Created    bool        `json:"created"`
	Unchanged  bool        `json:"unchanged,omitempty"`
	Superseded []string    `json:"superseded,omitempty"`
	Related    []Candidate `json:"related,omitempty"`
}

type ExistsError struct {
	Key          string
	Status       Status
	SupersededBy string
}

func (e *ExistsError) Error() string {
	msg := fmt.Sprintf("record %s already exists (%s)", e.Key, e.Status)
	if e.SupersededBy != "" {
		msg += ", superseded by " + e.SupersededBy
	}
	return msg + "; rerun with --update to change it in place, or pick a new key with --supersedes " + e.Key
}

type StatusError struct {
	Key          string
	Status       Status
	SupersededBy string
	Op           string
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("cannot %s %s: it is %s", e.Op, e.Key, e.Status)
	if e.SupersededBy != "" {
		msg += " and superseded by " + e.SupersededBy + "; change " + e.SupersededBy + " instead"
	}
	return msg
}

type GateError struct {
	Key        string
	Duplicates []Candidate
	Missing    []string
}

func (e *GateError) Error() string {
	keys := make([]string, len(e.Duplicates))
	for i, c := range e.Duplicates {
		keys[i] = c.Record.Key
	}
	msg := fmt.Sprintf("record %s looks like a duplicate of %s", e.Key, strings.Join(keys, ", "))
	if len(e.Missing) > 0 && len(e.Missing) < len(keys) {
		msg += "; --distinct does not list " + strings.Join(e.Missing, ", ")
	}
	return msg
}

func (s *Store) Put(ctx context.Context, req PutRequest) (PutResult, error) {
	if err := ValidKey(req.Key); err != nil {
		return PutResult{}, err
	}
	if req.Update && (len(req.Supersedes) > 0 || len(req.Distinct) > 0) {
		return PutResult{}, invalid("--update cannot be combined with --supersedes or --distinct")
	}
	if len(req.Distinct) > 0 && strings.TrimSpace(req.Why) == "" {
		return PutResult{}, invalid("--distinct needs --why")
	}
	if slices.Contains(req.Supersedes, req.Key) {
		return PutResult{}, invalid("a record cannot supersede itself")
	}
	req.Supersedes, req.Distinct = unique(req.Supersedes), unique(req.Distinct)
	g := req.Gate.withDefaults()
	var res PutResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if req.Update {
			res, err = s.update(ctx, tx, req, g)
		} else {
			res, err = s.create(ctx, tx, req, g)
		}
		return err
	})
	return res, err
}

func unique(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func apply(r *Record, req PutRequest) {
	if req.Kind != "" {
		r.Kind = req.Kind
	}
	if req.Scopes != nil {
		r.Scopes = keepTopics(r.Scopes, req.Scopes)
	}
	if req.Core != nil {
		r.Core = *req.Core
	}
	if req.Title != nil {
		r.Title = *req.Title
	}
	if req.Body != nil {
		r.Body = *req.Body
	}
	if req.Triggers != nil {
		r.Triggers = *req.Triggers
	}
	if req.VerifyCmd != nil {
		r.VerifyCmd = *req.VerifyCmd
	}
	if req.TTLDays != nil {
		r.TTLDays = *req.TTLDays
	}
	if req.Source != nil {
		r.Source = *req.Source
	}
}

func reason(why, fallback string) string {
	why = strings.TrimSpace(why)
	if why == "" {
		return fallback
	}
	return fallback + ": " + why
}

func (s *Store) update(ctx context.Context, tx *sql.Tx, req PutRequest, g Gate) (PutResult, error) {
	old, err := s.getRow(ctx, tx, req.Key)
	if errors.Is(err, ErrNotFound) {
		return PutResult{}, fmt.Errorf("%w; drop --update to create it", err)
	}
	if err != nil {
		return PutResult{}, err
	}
	if old.rec.Status != Current && old.rec.Status != Expired {
		return PutResult{}, &StatusError{Key: req.Key, Status: old.rec.Status, SupersededBy: old.rec.SupersededBy, Op: "update"}
	}
	next := old.rec
	apply(&next, req)
	if next.Kind != old.rec.Kind && req.TTLDays == nil {
		next.TTLDays = 0
	}
	next.Status = Current
	if err := next.normalize(); err != nil {
		return PutResult{}, err
	}
	res := PutResult{Key: next.Key}
	if sameContent(old.rec, next) && old.rec.Status == Current {
		res.Version = old.rec.Version
		res.Unchanged = true
		res.Related, err = s.related(ctx, tx, next, nil, g)
		return res, err
	}
	now := s.clock()
	if err := insertHistory(ctx, tx, old.rec, now, reason(req.Why, "update")); err != nil {
		return PutResult{}, err
	}
	next.Version = old.rec.Version + 1
	next.UpdatedAt = now
	if old.rec.Status == Current && sameClaim(old.rec, next) {
		next.UpdatedAt = old.rec.UpdatedAt
	}
	next.ValidTo = time.Time{}
	if err := updateRecord(ctx, tx, old.id, next); err != nil {
		return PutResult{}, err
	}
	if err := s.checkCore(ctx, tx, old.rec.Core || next.Core); err != nil {
		return PutResult{}, err
	}
	res.Version = next.Version
	res.Related, err = s.related(ctx, tx, next, nil, g)
	return res, err
}

func (s *Store) create(ctx context.Context, tx *sql.Tx, req PutRequest, g Gate) (PutResult, error) {
	if old, err := s.getRow(ctx, tx, req.Key); err == nil {
		return PutResult{}, &ExistsError{Key: req.Key, Status: old.rec.Status, SupersededBy: old.rec.SupersededBy}
	} else if !errors.Is(err, ErrNotFound) {
		return PutResult{}, err
	}
	rec := Record{Key: req.Key}
	apply(&rec, req)
	if err := rec.normalize(); err != nil {
		return PutResult{}, err
	}
	rec.Status = Current
	var olds []row
	for _, k := range req.Supersedes {
		o, err := s.getRow(ctx, tx, k)
		if err != nil {
			return PutResult{}, err
		}
		if o.rec.Status != Current && o.rec.Status != Expired {
			return PutResult{}, &StatusError{Key: k, Status: o.rec.Status, SupersededBy: o.rec.SupersededBy, Op: "supersede"}
		}
		olds = append(olds, o)
	}
	neigh, err := s.neighbours(ctx, tx, rec, req.Supersedes, g.Neighbours)
	if err != nil {
		return PutResult{}, err
	}
	var dups []Candidate
	if !g.Skip {
		for _, c := range neigh {
			if g.Duplicate(c) {
				dups = append(dups, c)
			}
		}
		var missing []string
		for _, c := range dups {
			if !slices.Contains(req.Distinct, c.Record.Key) {
				missing = append(missing, c.Record.Key)
			}
		}
		if len(missing) > 0 {
			return PutResult{}, &GateError{Key: rec.Key, Duplicates: dups, Missing: missing}
		}
	}
	now := s.clock()
	born := now
	if !req.AsOf.IsZero() && !req.AsOf.After(now) {
		born = req.AsOf.UTC().Truncate(time.Second)
	}
	rec.Version = 1
	rec.ValidFrom = born
	rec.CreatedAt = born
	rec.UpdatedAt = born
	if _, err := insertRecord(ctx, tx, rec); err != nil {
		return PutResult{}, err
	}
	for _, l := range req.Links {
		l.Src = rec.Key
		if err := addLink(ctx, tx, l); err != nil {
			return PutResult{}, err
		}
	}
	res := PutResult{Key: rec.Key, Version: 1, Created: true}
	coreTouched := rec.Core
	for _, o := range olds {
		if err := insertHistory(ctx, tx, o.rec, now, reason(req.Why, "superseded by "+rec.Key)); err != nil {
			return PutResult{}, err
		}
		next := o.rec
		next.Status = Superseded
		next.SupersededBy = rec.Key
		next.ValidTo = now
		next.Version++
		next.UpdatedAt = now
		if err := updateRecord(ctx, tx, o.id, next); err != nil {
			return PutResult{}, err
		}
		if err := addLink(ctx, tx, Link{Src: rec.Key, Dst: o.rec.Key, Rel: Supersedes}); err != nil {
			return PutResult{}, err
		}
		res.Superseded = append(res.Superseded, o.rec.Key)
		coreTouched = coreTouched || o.rec.Core
	}
	if len(req.Distinct) > 0 {
		note := "distinct: " + strings.TrimSpace(req.Why)
		for _, c := range dups {
			if err := addLink(ctx, tx, Link{Src: rec.Key, Dst: c.Record.Key, Rel: Relates, Note: note}); err != nil {
				return PutResult{}, err
			}
		}
	}
	if err := s.checkCore(ctx, tx, coreTouched); err != nil {
		return PutResult{}, err
	}
	res.Related = g.related(neigh)
	return res, nil
}

func (s *Store) related(ctx context.Context, q querier, rec Record, exclude []string, g Gate) ([]Candidate, error) {
	neigh, err := s.neighbours(ctx, q, rec, exclude, g.Neighbours)
	if err != nil {
		return nil, err
	}
	return g.related(neigh), nil
}

func (g Gate) Listed(c Candidate) bool {
	return c.Similarity >= g.Rel || c.Similarity >= min(g.Rel, RelFloor) && c.Topic >= RelTopic
}

func (g Gate) related(neigh []Candidate) []Candidate {
	var out []Candidate
	for _, c := range neigh {
		if g.Listed(c) {
			out = append(out, c)
		}
	}
	return out
}

func topicText(r Record) string {
	return KeyText(r.Key) + "\n" + r.Title
}

func (s *Store) Neighbours(ctx context.Context, rec Record, exclude []string, n int) ([]Candidate, error) {
	return s.neighbours(ctx, s.db, rec, exclude, n)
}

func (s *Store) neighbours(ctx context.Context, q querier, rec Record, exclude []string, n int) ([]Candidate, error) {
	if n <= 0 {
		n = DefaultNeighbours
	}
	scopes, err := NormalizeScopes(rec.Scopes)
	if err != nil {
		return nil, err
	}
	query := Query{
		Text:        RecordText(rec),
		Limit:       n,
		Kinds:       []Kind{rec.Kind},
		ExcludeKeys: append([]string{rec.Key}, exclude...),
		AllChunks:   true,
	}
	if !slices.Contains(scopes, GlobalScope) {
		query.Scopes = append(scopes, GlobalScope)
	}
	hits, err := s.search(ctx, q, query)
	if err != nil {
		return nil, err
	}
	self, topic := text.NewSet(RecordText(rec)), text.NewSet(topicText(rec))
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		other := text.NewSet(RecordText(h.Record))
		out = append(out, Candidate{
			Record:     h.Record,
			Similarity: self.Similarity(other),
			Shared:     self.Intersect(other),
			Topic:      topic.Intersect(text.NewSet(topicText(h.Record))),
			Snippet:    Snippet(h.Record.Body, "", SnippetLen),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Similarity > out[j].Similarity })
	return out, nil
}

func (s *Store) Retire(ctx context.Context, key, why string) (Record, error) {
	if strings.TrimSpace(why) == "" {
		return Record{}, invalid("retire needs --why")
	}
	var out Record
	err := s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.getRow(ctx, tx, key)
		if err != nil {
			return err
		}
		if old.rec.Status != Current && old.rec.Status != Expired {
			return &StatusError{Key: key, Status: old.rec.Status, SupersededBy: old.rec.SupersededBy, Op: "retire"}
		}
		now := s.clock()
		if err := insertHistory(ctx, tx, old.rec, now, reason(why, "retired")); err != nil {
			return err
		}
		next := old.rec
		next.Status = Retired
		next.ValidTo = now
		next.Version++
		next.UpdatedAt = now
		if err := updateRecord(ctx, tx, old.id, next); err != nil {
			return err
		}
		out = next
		return nil
	})
	return out, err
}

func (s *Store) Expire(ctx context.Context) ([]Record, error) {
	var out []Record
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := s.loadWhere(ctx, tx, "WHERE r.kind = ? AND r.status = ? ORDER BY r.key", State, Current)
		if err != nil {
			return err
		}
		now := s.clock()
		for _, rw := range rows {
			if now.Before(rw.rec.ExpiresAt()) {
				continue
			}
			reasonText := fmt.Sprintf("expired: unverified for %d days since %s", rw.rec.TTLDays, rw.rec.AsOf().Local().Format(time.DateOnly))
			if err := insertHistory(ctx, tx, rw.rec, now, reasonText); err != nil {
				return err
			}
			next := rw.rec
			next.Status = Expired
			next.ValidTo = now
			next.Version++
			next.UpdatedAt = now
			if err := updateRecord(ctx, tx, rw.id, next); err != nil {
				return err
			}
			out = append(out, next)
		}
		return nil
	})
	return out, err
}

func (s *Store) Verify(ctx context.Context, key, why string) (Record, error) {
	var out Record
	err := s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.getRow(ctx, tx, key)
		if err != nil {
			return err
		}
		if old.rec.Status != Current && old.rec.Status != Expired {
			return &StatusError{Key: key, Status: old.rec.Status, SupersededBy: old.rec.SupersededBy, Op: "verify"}
		}
		now := s.clock()
		next := old.rec
		next.VerifiedAt = now
		if old.rec.Status == Expired {
			if err := insertHistory(ctx, tx, old.rec, now, reason(why, "verified")); err != nil {
				return err
			}
			next.Status = Current
			next.ValidTo = time.Time{}
			next.Version++
			next.UpdatedAt = now
		}
		if err := updateRecord(ctx, tx, old.id, next); err != nil {
			return err
		}
		if err := s.checkCore(ctx, tx, next.Core); err != nil {
			return err
		}
		out = next
		return nil
	})
	return out, err
}

func sourceDate(r Record, now time.Time) time.Time {
	t := r.UpdatedAt
	if t.IsZero() {
		t = r.ValidFrom
	}
	if t.IsZero() || t.After(now) {
		return now
	}
	return t.UTC().Truncate(time.Second)
}

type UpsertResult struct {
	Created   []string `json:"created,omitempty"`
	Updated   []string `json:"updated,omitempty"`
	Unchanged int      `json:"unchanged"`
}

func (s *Store) Upsert(ctx context.Context, recs []Record, links []Link, why string) (UpsertResult, error) {
	var res UpsertResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		res = UpsertResult{}
		coreTouched := false
		now := s.clock()
		for _, rec := range recs {
			if err := rec.normalize(); err != nil {
				return err
			}
			asOf := sourceDate(rec, now)
			old, err := s.getRow(ctx, tx, rec.Key)
			if err == nil {
				rec.Scopes = keepTopics(old.rec.Scopes, rec.Scopes)
			}
			switch {
			case errors.Is(err, ErrNotFound):
				rec.Version = 1
				if rec.ValidFrom.IsZero() || rec.ValidFrom.After(now) {
					rec.ValidFrom = asOf
				}
				rec.CreatedAt = asOf
				rec.UpdatedAt = asOf
				if _, err := insertRecord(ctx, tx, rec); err != nil {
					return err
				}
				res.Created = append(res.Created, rec.Key)
				coreTouched = coreTouched || rec.Core
			case err != nil:
				return err
			case sameContent(old.rec, rec):
				res.Unchanged++
			default:
				if err := insertHistory(ctx, tx, old.rec, now, reason(why, "import")); err != nil {
					return err
				}
				next := old.rec
				next.Kind, next.Scopes, next.Core, next.Title, next.Body = rec.Kind, rec.Scopes, rec.Core, rec.Title, rec.Body
				next.Triggers, next.VerifyCmd, next.TTLDays, next.Source = rec.Triggers, rec.VerifyCmd, rec.TTLDays, rec.Source
				next.Version++
				next.UpdatedAt = asOf
				if !rec.ValidFrom.IsZero() && !rec.ValidFrom.After(now) {
					next.ValidFrom = rec.ValidFrom
				}
				if err := updateRecord(ctx, tx, old.id, next); err != nil {
					return err
				}
				res.Updated = append(res.Updated, rec.Key)
				coreTouched = coreTouched || rec.Core || old.rec.Core
			}
		}
		for _, l := range links {
			if err := addLink(ctx, tx, l); err != nil {
				return err
			}
		}
		return s.checkCore(ctx, tx, coreTouched)
	})
	return res, err
}

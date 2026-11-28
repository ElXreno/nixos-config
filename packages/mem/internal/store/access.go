package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

const AgentPrefix = "agent:"

type Access struct {
	Agent string
	See   []string
	All   bool
}

func AgentScope(name string) string { return AgentPrefix + strings.ToLower(strings.TrimSpace(name)) }

func ValidAgent(name string) error {
	if !scopeNameRe.MatchString(strings.ToLower(strings.TrimSpace(name))) {
		return invalid("agent %q is not a name of letters, digits and _.+@-", name)
	}
	return nil
}

func isAgentScope(s string) bool { return strings.HasPrefix(s, AgentPrefix) }

func AgentOf(scopes []string) string {
	for _, sc := range scopes {
		if isAgentScope(sc) {
			return sc[len(AgentPrefix):]
		}
	}
	return ""
}

func (r Record) Agent() string { return AgentOf(r.Scopes) }

func (a Access) normal() Access {
	a.Agent = strings.ToLower(strings.TrimSpace(a.Agent))
	see := make([]string, 0, len(a.See))
	for _, s := range a.See {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" && !slices.Contains(see, s) {
			see = append(see, s)
		}
	}
	a.See = see
	return a
}

func (a Access) validate() error {
	for _, name := range append([]string{a.Agent}, a.See...) {
		if name == "" {
			continue
		}
		if err := ValidAgent(name); err != nil {
			return err
		}
	}
	return nil
}

func (a Access) seesAll() bool { return a.All || a.Agent != "" }

func (a Access) Sees(scopes []string) bool {
	if a.seesAll() {
		return true
	}
	owner := AgentOf(scopes)
	return owner == "" || slices.Contains(a.See, owner)
}

func (a Access) Owns(scopes []string) bool {
	return a.All || AgentOf(scopes) == a.Agent
}

func (a Access) Borrowed() bool { return !a.All && a.Agent == "" && len(a.See) > 0 }

func (a Access) clause() string { return a.clauseAs("s") }

func (a Access) clauseAs(al string) string {
	if a.seesAll() {
		return ""
	}
	c := "NOT EXISTS (SELECT 1 FROM scopes " + al + " WHERE " + al + ".record_id = r.id AND substr(" + al + ".scope, 1, 6) = '" + AgentPrefix + "'"
	if len(a.See) > 0 {
		names := make([]string, len(a.See))
		for i, s := range a.See {
			names[i] = "'" + strings.ReplaceAll(AgentScope(s), "'", "''") + "'"
		}
		c += " AND " + al + ".scope NOT IN (" + strings.Join(names, ", ") + ")"
	}
	return c + ")"
}

func (a Access) scoped(where string) string {
	vis := a.clause()
	if vis == "" {
		return where
	}
	rest, order := where, ""
	if i := strings.LastIndex(where, " ORDER BY "); i >= 0 {
		rest, order = where[:i], where[i:]
	}
	if cond, ok := strings.CutPrefix(rest, "WHERE "); ok {
		return "WHERE " + vis + " AND (" + cond + ")" + order
	}
	return "WHERE " + vis + rest + order
}

func (f *filter) see(a Access) {
	if c := a.clause(); c != "" {
		f.add(c)
	}
}

type ForeignError struct {
	Key   string
	Owner string
	Agent string
	Op    string
}

func (e *ForeignError) Error() string {
	switch {
	case e.Op == "create":
		return fmt.Sprintf("key %s is taken by a record of another agent; pick another key", e.Key)
	case e.Owner == "":
		return fmt.Sprintf("cannot %s %s: it belongs to the shared memory, which agent %s only reads; write a record of your own under a new key and name %s in its body",
			e.Op, e.Key, e.Agent, e.Key)
	}
	return fmt.Sprintf("cannot %s %s: it belongs to agent %s", e.Op, e.Key, e.Owner)
}

func (s *Store) Access() Access { return s.access }

func (s *Store) visible() string {
	if c := s.access.clauseAs("own"); c != "" {
		return " AND " + c
	}
	return ""
}

func (s *Store) Unrestricted() *Store {
	c := *s
	c.access = Access{All: true}
	return &c
}

func (s *Store) owned(r Record, op string) error {
	if s.access.Borrowed() {
		return invalid("--as only reads; drop it to %s %s", op, r.Key)
	}
	if s.access.Owns(r.Scopes) {
		return nil
	}
	return &ForeignError{Key: r.Key, Owner: r.Agent(), Agent: s.access.Agent, Op: op}
}

func (s *Store) stamp(r *Record, keep string) error {
	if s.access.Borrowed() {
		return invalid("--as only reads; drop it to write %s", r.Key)
	}
	scopes := lowerAll(r.Scopes)
	given := AgentOf(scopes)
	want := s.access.Agent
	if keep != "" || s.access.All {
		want = keep
		if s.access.All && given != "" {
			want = given
		}
	}
	if given != "" && given != want {
		return invalid("scope %s%s is set by MEM_AGENT, not by --scope", AgentPrefix, given)
	}
	r.Scopes = slices.DeleteFunc(scopes, isAgentScope)
	if want != "" {
		r.Scopes = append(r.Scopes, AgentScope(want))
	}
	return nil
}

func (s *Store) Share(ctx context.Context, key, why string) (Record, error) {
	if strings.TrimSpace(why) == "" {
		return Record{}, invalid("share needs --why")
	}
	if s.access.Agent != "" {
		return Record{}, invalid("an agent cannot move its records into the shared memory; the owner does it with mem --as %s share %s", s.access.Agent, key)
	}
	var out Record
	all := s.Unrestricted()
	err := all.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.getRow(ctx, tx, key)
		if err != nil {
			return err
		}
		if old.rec.Agent() == "" {
			out = old.rec
			return nil
		}
		if old.rec.Status != Current && old.rec.Status != Expired {
			return &StatusError{Key: key, Status: old.rec.Status, SupersededBy: old.rec.SupersededBy, Op: "share"}
		}
		now := s.clock()
		if err := insertHistory(ctx, tx, old.rec, now, reason(why, "shared from agent "+old.rec.Agent())); err != nil {
			return err
		}
		next := old.rec
		next.Scopes = slices.DeleteFunc(slices.Clone(old.rec.Scopes), isAgentScope)
		if err := next.normalize(); err != nil {
			return err
		}
		next.Version++
		next.UpdatedAt = now
		if err := updateRecord(ctx, tx, old.id, next); err != nil {
			return err
		}
		out = next
		return all.checkCore(ctx, tx, next.Core)
	})
	return out, err
}

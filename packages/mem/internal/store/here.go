package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	hostPrefix    = "host:"
	projectPrefix = "project:"
)

type Here struct {
	Project       string
	Host          string
	StrictHost    bool
	StrictProject bool
}

func (h Here) local() []string {
	var out []string
	for _, sc := range []string{h.Project, h.Host} {
		if sc != "" {
			out = append(out, strings.ToLower(sc))
		}
	}
	return out
}

func (h Here) Scopes() []string {
	return append([]string{GlobalScope}, h.local()...)
}

func dimensionOK(scopes []string, prefix, want string) bool {
	has := false
	for _, sc := range scopes {
		if strings.HasPrefix(sc, prefix) {
			if sc == want {
				return true
			}
			has = true
		}
	}
	return !has
}

func (h Here) Matches(scopes []string) bool {
	if slices.Contains(scopes, GlobalScope) {
		return true
	}
	local := h.local()
	if !slices.ContainsFunc(scopes, func(sc string) bool { return slices.Contains(local, sc) }) {
		return false
	}
	if h.StrictHost && !dimensionOK(scopes, hostPrefix, strings.ToLower(h.Host)) {
		return false
	}
	return !h.StrictProject || dimensionOK(scopes, projectPrefix, strings.ToLower(h.Project))
}

func hasScope(cond string) string {
	return "EXISTS (SELECT 1 FROM scopes s WHERE s.record_id = r.id AND " + cond + ")"
}

func (h Here) clause() (string, []any) {
	global := hasScope("s.scope = ?")
	args := []any{GlobalScope}
	local := h.local()
	if len(local) == 0 {
		return global, args
	}
	inner := hasScope("s.scope IN (" + placeholders(len(local)) + ")")
	for _, sc := range local {
		args = append(args, sc)
	}
	dim := func(prefix, want string) {
		inner += " AND (NOT " + hasScope("substr(s.scope, 1, ?) = ?") + " OR " + hasScope("s.scope = ?") + ")"
		args = append(args, len(prefix), prefix, strings.ToLower(want))
	}
	if h.StrictHost {
		dim(hostPrefix, h.Host)
	}
	if h.StrictProject {
		dim(projectPrefix, h.Project)
	}
	return "(" + global + " OR (" + inner + "))", args
}

func (s *Store) CoreAt(ctx context.Context, h Here) ([]Record, error) {
	cond, args := h.clause()
	rows, err := s.loadWhere(ctx, s.db, "WHERE r.core = 1 AND r.status = ? AND "+cond+" ORDER BY r.key", append([]any{Current}, args...)...)
	if err != nil {
		return nil, err
	}
	out := make([]Record, len(rows))
	for i, r := range rows {
		out[i] = r.rec
	}
	return out, nil
}

const (
	StandingTopic  = "standing"
	StandingHeader = "Agent memory: standing rules, they hold on every turn; titles only, `mem get KEY` prints one in full.\n"
)

func (s *Store) StandingAt(ctx context.Context, h Here) ([]Record, error) {
	cond, args := h.clause()
	rows, err := s.loadWhere(ctx, s.db, "WHERE r.core = 0 AND r.status = ? AND "+hasScope("s.scope = ?")+" AND "+cond+" ORDER BY r.key",
		append([]any{Current, TopicScope(StandingTopic)}, args...)...)
	if err != nil {
		return nil, err
	}
	out := make([]Record, len(rows))
	for i, r := range rows {
		out[i] = r.rec
	}
	return out, nil
}

func RenderStanding(recs []Record, budget int) string {
	if len(recs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(StandingHeader)
	for i, r := range recs {
		line := "- `" + r.Key + "`: " + strings.TrimRight(r.Title, ".") + "\n"
		if budget > 0 && b.Len()+len(line) > budget {
			fmt.Fprintf(&b, "(+%d more: `mem list --scope %s`)\n", len(recs)-i, TopicScope(StandingTopic))
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func (s *Store) MatchTriggersAt(ctx context.Context, tool, subject string, h Here) ([]TriggerHit, error) {
	return s.matchTriggers(ctx, tool, subject, Query{Here: &h})
}

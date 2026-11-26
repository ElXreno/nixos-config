package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	TopicPrefix   = "topic:"
	MaxTitleRunes = 120
)

const schemaV4 = `
CREATE TABLE topics (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL
) WITHOUT ROWID;
`

const UsageHeader = "Memory lives in mem, not in the old memory files. Before acting on a topic run `mem search QUERY` or `mem get KEY`. " +
	"Write only through `mem put` (`--update` to change a record of the same topic, `--supersedes OLD` when a fact changed; " +
	"a state fact needs `--ttl` or `--verify`). Never edit the old memory files."

func TopicScope(id string) string { return TopicPrefix + strings.ToLower(id) }

func ValidTopicID(id string) error {
	if !scopeNameRe.MatchString(id) || len(id) > MaxKeyLen {
		return invalid("topic id %q must match [a-z0-9][a-z0-9_.+@-]* and be at most %d bytes", id, MaxKeyLen)
	}
	return nil
}

func isTopicScope(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), TopicPrefix)
}

func (r Record) Topic() string {
	for _, s := range r.Scopes {
		if id, ok := strings.CutPrefix(s, TopicPrefix); ok {
			return id
		}
	}
	return ""
}

func keepTopics(old, next []string) []string {
	if slices.ContainsFunc(next, isTopicScope) {
		return next
	}
	out := slices.Clone(next)
	for _, s := range old {
		if isTopicScope(s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

const schemaV5 = `
CREATE TABLE auto_topics (
	key TEXT PRIMARY KEY
) WITHOUT ROWID;
`

type TopicAssign struct {
	Key   string
	Topic string
	Title string
}

func (s *Store) AutoTopicKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM auto_topics`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

type TopicsResult struct {
	Topics    int      `json:"topics"`
	Titles    int      `json:"titles_changed"`
	Assigned  int      `json:"assigned"`
	Unchanged int      `json:"unchanged"`
	Unknown   []string `json:"unknown,omitempty"`
}

func cleanTitle(t string) (string, error) {
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		return "", invalid("a topic title must not be empty")
	}
	if utf8.RuneCountInString(t) > MaxTitleRunes {
		return "", invalid("topic title %q is longer than %d chars", t, MaxTitleRunes)
	}
	return t, nil
}

func (s *Store) SetTopics(ctx context.Context, as []TopicAssign) (TopicsResult, error) {
	return s.setTopics(ctx, as, false)
}

func (s *Store) SetAutoTopics(ctx context.Context, as []TopicAssign) (TopicsResult, error) {
	return s.setTopics(ctx, as, true)
}

func (s *Store) setTopics(ctx context.Context, as []TopicAssign, auto bool) (TopicsResult, error) {
	titles := map[string]string{}
	byKey := map[string]string{}
	norm := make([]TopicAssign, 0, len(as))
	for _, a := range as {
		if err := ValidKey(a.Key); err != nil {
			return TopicsResult{}, err
		}
		id := strings.ToLower(strings.TrimSpace(a.Topic))
		if err := ValidTopicID(id); err != nil {
			return TopicsResult{}, err
		}
		title, err := cleanTitle(a.Title)
		if err != nil {
			return TopicsResult{}, err
		}
		if prev, ok := titles[id]; ok && prev != title {
			return TopicsResult{}, invalid("topic %s has two titles: %q and %q", id, prev, title)
		}
		titles[id] = title
		if prev, ok := byKey[a.Key]; ok && prev != id {
			return TopicsResult{}, invalid("record %s is mapped to two topics: %s and %s", a.Key, prev, id)
		}
		byKey[a.Key] = id
		norm = append(norm, TopicAssign{Key: a.Key, Topic: id, Title: title})
	}
	var res TopicsResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		res = TopicsResult{Topics: len(titles)}
		ids := make([]string, 0, len(titles))
		for id := range titles {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			r, err := tx.ExecContext(ctx, `INSERT INTO topics (id, title) VALUES (?, ?)
				ON CONFLICT (id) DO UPDATE SET title = excluded.title WHERE title <> excluded.title`, id, titles[id])
			if err != nil {
				return err
			}
			if n, _ := r.RowsAffected(); n > 0 {
				res.Titles++
			}
		}
		done := map[string]bool{}
		for _, a := range norm {
			if done[a.Key] {
				continue
			}
			done[a.Key] = true
			rw, err := s.getRow(ctx, tx, a.Key)
			if errors.Is(err, ErrNotFound) {
				res.Unknown = append(res.Unknown, a.Key)
				continue
			}
			if err != nil {
				return err
			}
			mark := `DELETE FROM auto_topics WHERE key = ?`
			if auto {
				mark = `INSERT OR IGNORE INTO auto_topics (key) VALUES (?)`
			}
			if _, err := tx.ExecContext(ctx, mark, a.Key); err != nil {
				return err
			}
			want := TopicScope(a.Topic)
			var have []string
			for _, sc := range rw.rec.Scopes {
				if isTopicScope(sc) {
					have = append(have, sc)
				}
			}
			if len(have) == 1 && have[0] == want {
				res.Unchanged++
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM scopes WHERE record_id = ? AND substr(scope, 1, 6) = ?`, rw.id, TopicPrefix); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO scopes (record_id, scope) VALUES (?, ?)`, rw.id, want); err != nil {
				return err
			}
			res.Assigned++
		}
		return nil
	})
	return res, err
}

type TopicInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Records int    `json:"records"`
	Logs    int    `json:"logs"`
}

func (s *Store) Topics(ctx context.Context) ([]TopicInfo, error) {
	by := map[string]*TopicInfo{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, title FROM topics`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t TopicInfo
		if err := rows.Scan(&t.ID, &t.Title); err != nil {
			_ = rows.Close()
			return nil, err
		}
		by[t.ID] = &t
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	counts, err := s.db.QueryContext(ctx, `SELECT substr(s.scope, 7), r.kind = 'log', count(*)
		FROM scopes s JOIN records r ON r.id = s.record_id
		WHERE substr(s.scope, 1, 6) = ? AND r.status = ? GROUP BY 1, 2`, TopicPrefix, Current)
	if err != nil {
		return nil, err
	}
	defer func() { _ = counts.Close() }()
	for counts.Next() {
		var id string
		var log, n int
		if err := counts.Scan(&id, &log, &n); err != nil {
			return nil, err
		}
		t := by[id]
		if t == nil {
			t = &TopicInfo{ID: id}
			by[id] = t
		}
		if log == 1 {
			t.Logs = n
		} else {
			t.Records = n
		}
	}
	if err := counts.Err(); err != nil {
		return nil, err
	}
	out := make([]TopicInfo, 0, len(by))
	for _, t := range by {
		out = append(out, *t)
	}
	slices.SortFunc(out, func(a, b TopicInfo) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

type TopicEntry struct {
	ID       string
	Title    string
	Records  int
	Keywords []string
}

func (s *Store) pastTTL(ctx context.Context) (map[string]bool, error) {
	rows, err := s.loadWhere(ctx, s.db, "WHERE r.kind = ? AND r.status = ?", State, Current)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	out := map[string]bool{}
	for _, rw := range rows {
		if !now.Before(rw.rec.ExpiresAt()) {
			out[rw.rec.Key] = true
		}
	}
	return out, nil
}

func (s *Store) TopicMap(ctx context.Context, keywords int) ([]TopicEntry, error) {
	infos, err := s.Topics(ctx)
	if err != nil {
		return nil, err
	}
	stale, err := s.pastTTL(ctx)
	if err != nil {
		return nil, err
	}
	titles := make(map[string]string, len(infos))
	for _, t := range infos {
		titles[t.ID] = t.Title
	}
	rows, err := s.db.QueryContext(ctx, `SELECT substr(s.scope, 7), r.key, r.title
		FROM scopes s JOIN records r ON r.id = s.record_id
		WHERE substr(s.scope, 1, 6) = ? AND r.status = ? AND r.kind <> 'log'
		ORDER BY s.scope, r.key`, TopicPrefix, Current)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	by := map[string]*TopicEntry{}
	docs := map[string][]string{}
	for rows.Next() {
		var id, key, title string
		if err := rows.Scan(&id, &key, &title); err != nil {
			return nil, err
		}
		if stale[key] {
			continue
		}
		e := by[id]
		if e == nil {
			e = &TopicEntry{ID: id, Title: titles[id]}
			by[id] = e
		}
		e.Records++
		docs[id] = append(docs[id], title)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	kws := topicKeywords(docs, titles, keywords)
	out := make([]TopicEntry, 0, len(by))
	for id, e := range by {
		e.Keywords = kws[id]
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b TopicEntry) int {
		if a.Records != b.Records {
			return b.Records - a.Records
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

const mapHeader = "Memory topics (title [id] current records: keywords of their titles); `mem search QUERY --scope topic:ID` reads one, `mem topics list` all:\n"

func runes(s string) int { return utf8.RuneCountInString(s) }

func topicLine(e TopicEntry, keywords int) string {
	var b strings.Builder
	b.WriteString("- ")
	if e.Title != "" {
		fmt.Fprintf(&b, "%s [%s] %d", e.Title, e.ID, e.Records)
	} else {
		fmt.Fprintf(&b, "%s %d", e.ID, e.Records)
	}
	if n := min(keywords, len(e.Keywords)); n > 0 {
		b.WriteString(": " + strings.Join(e.Keywords[:n], ", "))
	}
	b.WriteString("\n")
	return b.String()
}

func trimKeywords(es []TopicEntry, budget int) ([]int, bool) {
	keep := make([]int, len(es))
	lens := make([]int, len(es))
	total, top := runes(mapHeader), 0
	for i, e := range es {
		keep[i] = len(e.Keywords)
		top = max(top, keep[i])
		lens[i] = runes(topicLine(e, keep[i]))
		total += lens[i]
	}
	for level := top; level > 0 && total > budget; level-- {
		for i := range es {
			if keep[i] < level {
				continue
			}
			keep[i]--
			n := runes(topicLine(es[i], keep[i]))
			total, lens[i] = total+n-lens[i], n
			if total <= budget {
				break
			}
		}
	}
	return keep, total <= budget
}

func compactList(es []TopicEntry, room int) (string, bool) {
	line := ""
	for i, e := range es {
		cand := line
		if cand != "" {
			cand += ", "
		}
		cand += fmt.Sprintf("%s %d", e.ID, e.Records)
		suffix := "\n"
		if rest := len(es) - i - 1; rest > 0 {
			suffix = fmt.Sprintf(", +%d more\n", rest)
		}
		if runes(cand)+runes(suffix) > room {
			if line == "" {
				return "", false
			}
			return line + fmt.Sprintf(", +%d more\n", len(es)-i), false
		}
		line = cand
	}
	return line + "\n", true
}

func RenderTopicMap(es []TopicEntry, budget int) string {
	if len(es) == 0 || budget <= runes(mapHeader) {
		return ""
	}
	if keep, ok := trimKeywords(es, budget); ok {
		var b strings.Builder
		b.WriteString(mapHeader)
		for i, e := range es {
			b.WriteString(topicLine(e, keep[i]))
		}
		if out := b.String(); runes(out) <= budget {
			return out
		}
	}
	for titled := len(es) - 1; titled >= 0; titled-- {
		var b strings.Builder
		b.WriteString(mapHeader)
		for _, e := range es[:titled] {
			b.WriteString(topicLine(e, 0))
		}
		rest, whole := compactList(es[titled:], budget-runes(b.String()))
		if rest == "" || (!whole && titled > 0) {
			continue
		}
		return b.String() + rest
	}
	return ""
}

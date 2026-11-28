package store

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mem/internal/text"
)

type Mode int

const (
	Fused Mode = iota
	WordsOnly
	TrigramOnly
)

func (m Mode) String() string {
	switch m {
	case WordsOnly:
		return "words"
	case TrigramOnly:
		return "trigram"
	}
	return "fused"
}

const (
	RRFK         = 60
	TriWeight    = 0.3
	MaxTerms     = 64
	MaxTriTerms  = 24
	MinTriStem   = 5
	DefaultLimit = 10
	MinDepth     = 50
	SnippetLen   = 200
)

type Query struct {
	Text         string
	Limit        int
	Depth        int
	Kinds        []Kind
	ExcludeKinds []Kind
	Scopes       []string
	Statuses     []Status
	AllStatus    bool
	ExcludeKeys  []string
	Mode         Mode
	AllChunks    bool
	Here         *Here
	Fallback     bool
	Before       time.Time
}

type Hit struct {
	Record    Record  `json:"record"`
	Score     float64 `json:"score"`
	WordsRank int     `json:"words_rank,omitempty"`
	TriRank   int     `json:"tri_rank,omitempty"`
	WordsBM25 float64 `json:"words_bm25,omitempty"`
	TriBM25   float64 `json:"tri_bm25,omitempty"`
	Overlap   float64 `json:"overlap"`
	Snippet   string  `json:"snippet"`
}

func ftsQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func pickTerms(terms []string, limit int) []string {
	if len(terms) <= limit {
		return terms
	}
	idx := make([]int, len(terms))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return utf8.RuneCountInString(terms[idx[a]]) > utf8.RuneCountInString(terms[idx[b]])
	})
	idx = idx[:limit]
	slices.Sort(idx)
	out := make([]string, len(idx))
	for i, j := range idx {
		out[i] = terms[j]
	}
	return out
}

func (q Query) terms() []string {
	t := text.Terms(q.Text)
	if len(t) > 0 || !q.Fallback {
		return t
	}
	if t = text.LooseTerms(q.Text); len(t) > 0 {
		return t
	}
	return text.RawTerms(q.Text)
}

func WordsQuery(q string) string {
	return wordsQuery(text.Terms(q))
}

func wordsQuery(terms []string) string {
	var parts []string
	for _, t := range pickTerms(terms, MaxTerms) {
		if utf8.RuneCountInString(t) >= 3 {
			parts = append(parts, ftsQuote(t)+"*")
		} else {
			parts = append(parts, ftsQuote(t))
		}
	}
	return strings.Join(parts, " OR ")
}

func isEdgePunct(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func technical(chunk string) bool {
	lower := false
	for _, r := range chunk {
		if unicode.IsDigit(r) || strings.ContainsRune("_.:/-#@", r) || lower && unicode.IsUpper(r) {
			return true
		}
		lower = unicode.IsLower(r)
	}
	return false
}

func TrigramQuery(q string, stems bool) string {
	return trigramQuery(q, text.Terms(q), stems)
}

func trigramQuery(q string, stemTerms []string, stems bool) string {
	seen := map[string]bool{}
	var terms []string
	add := func(t string) {
		if utf8.RuneCountInString(t) < 3 || seen[t] {
			return
		}
		seen[t] = true
		terms = append(terms, t)
	}
	for _, f := range strings.Fields(q) {
		chunk := strings.TrimFunc(f, isEdgePunct)
		if technical(chunk) {
			add(strings.ReplaceAll(strings.ToLower(chunk), "ё", "е"))
		}
	}
	if stems {
		for _, t := range stemTerms {
			if utf8.RuneCountInString(t) >= MinTriStem {
				add(t)
			}
		}
	}
	terms = pickTerms(terms, MaxTriTerms)
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = ftsQuote(t)
	}
	return strings.Join(parts, " OR ")
}

type filter struct {
	where []string
	args  []any
}

func (f *filter) add(cond string, args ...any) {
	f.where = append(f.where, cond)
	f.args = append(f.args, args...)
}

func (f *filter) in(col string, vals []string, not bool) {
	if len(vals) == 0 {
		return
	}
	op := " IN ("
	if not {
		op = " NOT IN ("
	}
	args := make([]any, len(vals))
	for i, v := range vals {
		args[i] = v
	}
	f.add(col+op+placeholders(len(vals))+")", args...)
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

func (q Query) filter() filter {
	var f filter
	if !q.AllStatus {
		st := q.Statuses
		if len(st) == 0 {
			st = []Status{Current}
		}
		f.in("r.status", strs(st), false)
	}
	f.in("r.kind", strs(q.Kinds), false)
	f.in("r.kind", strs(q.ExcludeKinds), true)
	f.in("r.key", q.ExcludeKeys, true)
	if len(q.Scopes) > 0 {
		sc := lowerAll(q.Scopes)
		args := make([]any, len(sc))
		for i, v := range sc {
			args[i] = v
		}
		f.add("EXISTS (SELECT 1 FROM scopes s WHERE s.record_id = r.id AND s.scope IN ("+placeholders(len(sc))+"))", args...)
	}
	if q.Here != nil {
		cond, args := q.Here.clause()
		f.add(cond, args...)
	}
	if !q.Before.IsZero() {
		f.add("r.updated_at <= ?", fmtTime(q.Before))
	}
	return f
}

func (f filter) sql() string {
	if len(f.where) == 0 {
		return ""
	}
	return " AND " + strings.Join(f.where, " AND ")
}

type ranked struct {
	id    int64
	key   string
	score float64
}

func rankFTS(ctx context.Context, q querier, table, match string, f filter, depth int) ([]ranked, error) {
	if match == "" {
		return nil, nil
	}
	args := append([]any{match}, f.args...)
	args = append(args, depth)
	rows, err := q.QueryContext(ctx, `SELECT r.id, r.key, -bm25(`+table+`, 2.0, 2.0, 1.0) AS score FROM `+table+`
		JOIN records r ON r.id = `+table+`.rowid
		WHERE `+table+` MATCH ?`+f.sql()+`
		ORDER BY score DESC, r.key LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ranked
	for rows.Next() {
		var r ranked
		if err := rows.Scan(&r.id, &r.key, &r.score); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type ListHit struct {
	Key   string
	Rank  int
	Score float64
}

func (s *Store) Lists(ctx context.Context, q Query) (words, tri []ListHit, err error) {
	depth := q.Depth
	if depth <= 0 {
		depth = max(MinDepth, 5*max(q.Limit, DefaultLimit))
	}
	f := q.filter()
	f.see(s.access)
	terms := q.terms()
	conv := func(in []ranked) []ListHit {
		out := make([]ListHit, len(in))
		for i, r := range in {
			out[i] = ListHit{Key: r.key, Rank: i + 1, Score: r.score}
		}
		return out
	}
	if q.Mode != TrigramOnly {
		rs, err := rankFTS(ctx, s.db, "fts_words", wordsQuery(terms), f, depth)
		if err != nil {
			return nil, nil, err
		}
		words = conv(rs)
	}
	if q.Mode != WordsOnly {
		rs, err := rankFTS(ctx, s.db, "fts_tri", trigramQuery(q.Text, terms, q.Mode == TrigramOnly), f, depth)
		if err != nil {
			return nil, nil, err
		}
		tri = conv(rs)
	}
	return words, tri, nil
}

func (s *Store) Search(ctx context.Context, q Query) ([]Hit, error) {
	return s.search(ctx, s.db, q)
}

func (s *Store) search(ctx context.Context, db querier, q Query) ([]Hit, error) {
	if q.Limit <= 0 {
		q.Limit = DefaultLimit
	}
	depth := q.Depth
	if depth <= 0 {
		depth = max(MinDepth, 5*q.Limit)
	}
	f := q.filter()
	f.see(s.access)
	terms := q.terms()
	type acc struct {
		hit Hit
		id  int64
	}
	byID := map[int64]*acc{}
	get := func(id int64) *acc {
		a := byID[id]
		if a == nil {
			a = &acc{id: id}
			byID[id] = a
		}
		return a
	}
	if q.Mode != TrigramOnly {
		words, err := rankFTS(ctx, db, "fts_words", wordsQuery(terms), f, depth)
		if err != nil {
			return nil, err
		}
		for i, r := range words {
			a := get(r.id)
			a.hit.WordsRank = i + 1
			a.hit.WordsBM25 = r.score
			a.hit.Score += 1 / float64(RRFK+i+1)
		}
	}
	if q.Mode != WordsOnly {
		tri, err := rankFTS(ctx, db, "fts_tri", trigramQuery(q.Text, terms, q.Mode == TrigramOnly), f, depth)
		if err != nil {
			return nil, err
		}
		weight := 1.0
		if q.Mode == Fused {
			weight = TriWeight
		}
		for i, r := range tri {
			a := get(r.id)
			a.hit.TriRank = i + 1
			a.hit.TriBM25 = r.score
			a.hit.Score += weight / float64(RRFK+i+1)
		}
	}
	if len(byID) == 0 {
		return nil, nil
	}
	all := make([]*acc, 0, len(byID))
	for _, a := range byID {
		all = append(all, a)
	}
	ids := make([]any, len(all))
	for i, a := range all {
		ids[i] = a.id
	}
	rows, err := s.loadWhere(ctx, db, "WHERE r.id IN ("+placeholders(len(ids))+")", ids...)
	if err != nil {
		return nil, err
	}
	for _, rw := range rows {
		byID[rw.id].hit.Record = rw.rec
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i].hit, all[j].hit
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.WordsBM25+a.TriBM25 != b.WordsBM25+b.TriBM25 {
			return a.WordsBM25+a.TriBM25 > b.WordsBM25+b.TriBM25
		}
		return a.Record.Key < b.Record.Key
	})
	if !q.AllChunks {
		seen := map[string]bool{}
		kept := all[:0]
		for _, a := range all {
			if src := chunkSource(a.hit.Record); !seen[src] {
				seen[src] = true
				kept = append(kept, a)
			}
		}
		all = kept
	}
	if len(all) > q.Limit {
		all = all[:q.Limit]
	}
	out := make([]Hit, len(all))
	for i, a := range all {
		h := a.hit
		h.Overlap = text.Coverage(terms, RecordText(h.Record))
		h.Snippet = snippetOf(h.Record.Body, terms, SnippetLen)
		out[i] = h
	}
	return out, nil
}

func chunkSource(r Record) string {
	if r.Kind == Log {
		if i := strings.LastIndexByte(r.Key, '#'); i > 0 {
			return r.Key[:i]
		}
	}
	return r.Key
}

var legacyPrefixes = []string{"feedback", "reference", "project", "user"}

func KeyText(key string) string {
	parts := strings.FieldsFunc(key, func(r rune) bool { return strings.ContainsRune("_.:#-", r) })
	if len(parts) > 1 && slices.Contains(legacyPrefixes, parts[0]) {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}

func RecordText(r Record) string {
	return KeyText(r.Key) + "\n" + r.Title + "\n" + r.Body
}

func Snippet(body, query string, n int) string {
	return snippetOf(body, text.Terms(query), n)
}

func snippetOf(body string, terms []string, n int) string {
	flat, start, end := snippetWindow(body, terms, n)
	runes := []rune(flat)
	out := string(runes[start:end])
	if start > 0 {
		out = "..." + out
	}
	if end < len(runes) {
		out += "..."
	}
	return out
}

func SnippetWindow(body, query string, n int) (string, int, int) {
	return snippetWindow(body, text.Terms(query), n)
}

func snippetWindow(body string, terms []string, n int) (string, int, int) {
	flat := strings.Join(strings.Fields(body), " ")
	runes := []rune(flat)
	if len(runes) <= n {
		return flat, 0, len(runes)
	}
	start := 0
	if len(terms) > 0 {
		lower := strings.ReplaceAll(strings.ToLower(flat), "ё", "е")
		best := -1
		for _, t := range terms {
			if i := strings.Index(lower, t); i >= 0 && (best < 0 || i < best) {
				best = i
			}
		}
		if best > 0 {
			start = max(0, utf8.RuneCountInString(lower[:best])-n/4)
		}
	}
	end := min(len(runes), start+n)
	if end-start < n {
		start = max(0, end-n)
	}
	return flat, start, end
}

func (q Query) Snippet(body string, n int) string {
	return snippetOf(body, q.terms(), n)
}

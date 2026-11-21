package store

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"mem/internal/text"
)

const (
	MapKeywords     = 5
	minKeywordRunes = 3
	keywordJoiners  = "-_."
)

func keywordTokens(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.Is(unicode.Mn, r) && !strings.ContainsRune(keywordJoiners, r)
	}) {
		if f = strings.Trim(f, keywordJoiners); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func foldTerm(tok string) string {
	return strings.Map(func(r rune) rune {
		if r = unicode.ToLower(r); r == 'ё' {
			return 'е'
		}
		return r
	}, tok)
}

func usableTerm(tok, term string) bool {
	return utf8.RuneCountInString(tok) >= minKeywordRunes && !text.IsStopword(term) && strings.IndexFunc(tok, unicode.IsLetter) >= 0
}

func topicWords(id, title string) map[string]bool {
	out := map[string]bool{foldTerm(id): true}
	for _, part := range strings.FieldsFunc(id, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		out[foldTerm(part)] = true
	}
	for _, tok := range keywordTokens(title) {
		out[foldTerm(tok)] = true
	}
	return out
}

type termStat struct {
	tf    int
	forms map[string]int
}

func (t *termStat) form() string {
	best, n := "", 0
	for f, c := range t.forms {
		if c > n || (c == n && f < best) {
			best, n = f, c
		}
	}
	return best
}

func topicKeywords(docs map[string][]string, names map[string]string, n int) map[string][]string {
	out := map[string][]string{}
	if n <= 0 {
		return out
	}
	stats := make(map[string]map[string]*termStat, len(docs))
	df := map[string]int{}
	for id, titles := range docs {
		st := map[string]*termStat{}
		for _, title := range titles {
			for _, tok := range keywordTokens(title) {
				term := foldTerm(tok)
				if !usableTerm(tok, term) {
					continue
				}
				ts := st[term]
				if ts == nil {
					ts = &termStat{forms: map[string]int{}}
					st[term] = ts
				}
				ts.tf++
				ts.forms[tok]++
			}
		}
		for term := range st {
			df[term]++
		}
		stats[id] = st
	}
	topics := float64(len(docs))
	type cand struct {
		term  string
		score float64
		tf    int
	}
	for id, st := range stats {
		skip := topicWords(id, names[id])
		var cs []cand
		for term, ts := range st {
			if !skip[term] {
				cs = append(cs, cand{term: term, score: float64(ts.tf) * math.Log(topics/float64(df[term])), tf: ts.tf})
			}
		}
		slices.SortFunc(cs, func(a, b cand) int {
			if c := cmp.Compare(b.score, a.score); c != 0 {
				return c
			}
			if a.tf != b.tf {
				return b.tf - a.tf
			}
			return strings.Compare(a.term, b.term)
		})
		for _, c := range cs[:min(n, len(cs))] {
			out[id] = append(out[id], st[c.term].form())
		}
	}
	return out
}

package text

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Mn, r)
}

func fold(r rune) rune {
	r = unicode.ToLower(r)
	if r == 'ё' {
		return 'е'
	}
	return r
}

func Tokens(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if isWordRune(r) {
			if !unicode.Is(unicode.Mn, r) {
				b.WriteRune(fold(r))
			}
			continue
		}
		flush()
	}
	flush()
	return out
}

func script(tok string) (cyrillic, latin, digit bool) {
	for _, r := range tok {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic = true
		case r < utf8.RuneSelf && unicode.IsLetter(r):
			latin = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	return
}

func Stem(tok string) string {
	cyr, lat, dig := script(tok)
	switch {
	case dig:
		return tok
	case cyr && !lat:
		return string(stemRu([]rune(tok)))
	case lat && !cyr:
		return stemEn(tok)
	}
	return tok
}

func Informative(tok string) bool {
	return utf8.RuneCountInString(tok) > 1 && !stopwords[tok]
}

func Terms(s string) []string {
	return stems(s, Informative)
}

func LooseTerms(s string) []string {
	return stems(s, func(tok string) bool { return utf8.RuneCountInString(tok) > 1 && !searchStopwords[tok] })
}

func RawTerms(s string) []string {
	return stems(s, func(string) bool { return true })
}

func stems(s string, keep func(string) bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, tok := range Tokens(s) {
		if !keep(tok) {
			continue
		}
		st := Stem(tok)
		if st == "" || seen[st] {
			continue
		}
		seen[st] = true
		out = append(out, st)
	}
	return out
}

type Set map[string]struct{}

func NewSet(s string) Set {
	set := Set{}
	for _, t := range Terms(s) {
		set[t] = struct{}{}
	}
	return set
}

func (a Set) Intersect(b Set) int {
	if len(b) < len(a) {
		a, b = b, a
	}
	n := 0
	for t := range a {
		if _, ok := b[t]; ok {
			n++
		}
	}
	return n
}

func Similarity(a, b string) float64 {
	return NewSet(a).Similarity(NewSet(b))
}

func (a Set) Similarity(b Set) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := float64(a.Intersect(b))
	dice := 2 * inter / float64(len(a)+len(b))
	cover := inter / math.Min(float64(len(a)), float64(len(b)))
	return math.Sqrt(dice * cover)
}

func Normalize(s string) string {
	return strings.Join(Tokens(s), " ")
}

func Coverage(terms []string, doc string) float64 {
	if len(terms) == 0 {
		return 0
	}
	norm := " " + Normalize(doc) + " "
	hit := 0
	for _, t := range terms {
		if utf8.RuneCountInString(t) >= 3 {
			if strings.Contains(norm, t) {
				hit++
			}
		} else if strings.Contains(norm, " "+t+" ") {
			hit++
		}
	}
	return float64(hit) / float64(len(terms))
}

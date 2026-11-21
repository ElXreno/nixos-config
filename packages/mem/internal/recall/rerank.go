package recall

import (
	"slices"
	"unicode/utf8"

	"mem/internal/store"
)

const (
	DefaultRerankText       = 0.99
	DefaultRerankPointer    = 0.9
	DefaultRerankTake       = 4
	DefaultRerankCandidates = 12
	DefaultRerankDocChars   = 800
	DefaultRerankQueryChars = 1000
)

type RerankGate struct {
	Text    float64 `json:"text"`
	Pointer float64 `json:"pointer"`
	Take    int     `json:"take"`
}

func DefaultRerankGate() RerankGate {
	return RerankGate{Text: DefaultRerankText, Pointer: DefaultRerankPointer, Take: DefaultRerankTake}
}

type Reranked struct {
	VHit
	Rerank float64 `json:"rerank"`
}

func ClipRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func RerankDoc(r store.Record, chars int) string {
	body := ClipRunes(r.Body, chars)
	if r.Title == "" {
		return body
	}
	return r.Title + "\n" + body
}

func (v VectorSettings) rerankStrongKind(log bool) bool {
	if log {
		return v.Log.Take > 0
	}
	return v.Reg.Take > 0 && !v.LexRegular
}

func (v VectorSettings) rerankPointerKind(log bool) bool {
	if log {
		return v.Log.Take > 0 || v.Log.PointerCos > 0
	}
	return v.Reg.Take > 0 || v.Reg.PointerCos > 0
}

func (v VectorSettings) RerankCandidates(built []VHit, g RerankGate, n int) []VHit {
	var out []VHit
	for _, h := range built {
		if len(out) == n {
			break
		}
		if g.Take > 0 && v.rerankStrongKind(h.Log) || g.Pointer > 0 && v.PointerTake > 0 && v.rerankPointerKind(h.Log) {
			out = append(out, h)
		}
	}
	return out
}

func byRerank(list []Reranked) []Reranked {
	out := slices.Clone(list)
	slices.SortStableFunc(out, func(a, b Reranked) int {
		switch {
		case a.Rerank > b.Rerank:
			return -1
		case a.Rerank < b.Rerank:
			return 1
		}
		return 0
	})
	return out
}

func (v VectorSettings) RerankStrong(g RerankGate, list []Reranked) Selection {
	var sel Selection
	n := 0
	for _, h := range byRerank(list) {
		if n >= g.Take {
			break
		}
		if h.Rerank < g.Text || h.Expired || !v.rerankStrongKind(h.Log) {
			continue
		}
		if h.Log {
			sel.Log = append(sel.Log, h.VHit)
		} else {
			sel.Reg = append(sel.Reg, h.VHit)
		}
		n++
	}
	return sel
}

func (v VectorSettings) RerankPointers(g RerankGate, list []Reranked, skip func(VHit) bool) Selection {
	var sel Selection
	if g.Pointer <= 0 || v.PointerTake <= 0 {
		return sel
	}
	sorted := byRerank(list)
	for _, h := range sorted {
		if h.Expired || h.Rerank < g.Pointer || !v.rerankPointerKind(h.Log) || skip != nil && skip(h.VHit) {
			continue
		}
		if h.Log && len(sel.Log) < v.PointerTake {
			sel.Log = append(sel.Log, h.VHit)
		} else if !h.Log && len(sel.Reg) < v.PointerTake {
			sel.Reg = append(sel.Reg, h.VHit)
		}
	}
	n := 0
	for _, h := range sorted {
		if n == ExpiredCandidates {
			break
		}
		if !h.Expired || h.Log || h.Rerank < g.Pointer || !v.rerankPointerKind(false) || skip != nil && skip(h.VHit) {
			continue
		}
		sel.Reg = append(sel.Reg, h.VHit)
		n++
	}
	return sel
}

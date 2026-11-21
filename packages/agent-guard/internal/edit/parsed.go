package edit

import (
	"maps"
	"slices"
	"sort"
	"strings"

	"agent-guard/internal/call"
	"agent-guard/internal/treesitter"
)

type span struct{ start, end, pair int }

// applyEdit replays the change on the file the way the tool applies it.
func applyEdit(c call.Change, opts Options) (string, []span, []string, bool) {
	if c.Write {
		return c.Content, []span{{0, len(c.Content), 0}}, []string{opts.previous(c.Path)}, true
	}
	text, ok := opts.read(c.Path)
	if !ok || len(c.Pairs) == 0 {
		return "", nil, nil, false
	}
	pairs := c.Pairs
	if c.Apply == call.Original {
		text = toLF(strings.TrimPrefix(text, "\uFEFF"))
		pairs = make([]call.Pair, len(c.Pairs))
		for k, p := range c.Pairs {
			pairs[k] = call.Pair{Old: toLF(p.Old), New: toLF(p.New), All: p.All}
		}
	}
	olds := make([]string, len(pairs))
	for k, p := range pairs {
		olds[k] = p.Old
	}
	var spans []span
	if c.Apply == call.Original {
		if text, spans, ok = applyOriginal(text, pairs); !ok {
			opts.note("no exact match for the edit in " + c.Path)
		}
	} else {
		text, spans, ok = applySequential(text, pairs)
	}
	if !ok {
		return "", nil, nil, false
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].start < spans[b].start })
	return text, spans, olds, true
}

func take(kept map[string]int, key string) bool {
	if kept[key] == 0 {
		return false
	}
	kept[key]--
	return true
}

func toLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func occurrences(text string, p call.Pair) ([]int, bool) {
	if p.Old == "" {
		return nil, false
	}
	var at []int
	for from := 0; ; {
		i := strings.Index(text[from:], p.Old)
		if i < 0 {
			break
		}
		at = append(at, from+i)
		from += i + len(p.Old)
	}
	return at, len(at) == 1 || (len(at) > 1 && p.All)
}

func applySequential(text string, pairs []call.Pair) (string, []span, bool) {
	var spans []span
	for k, p := range pairs {
		at, ok := occurrences(text, p)
		if !ok {
			return "", nil, false
		}
		for _, pos := range slices.Backward(at) {
			if text, spans, ok = replaceAt(text, spans, pos, p.Old, p.New, k); !ok {
				return "", nil, false
			}
		}
	}
	return text, spans, true
}

func applyOriginal(text string, pairs []call.Pair) (string, []span, bool) {
	type hit struct{ pos, pair int }
	var hits []hit
	for k, p := range pairs {
		at, ok := occurrences(text, p)
		if !ok {
			return "", nil, false
		}
		for _, pos := range at {
			hits = append(hits, hit{pos, k})
		}
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].pos < hits[b].pos })
	for i := 1; i < len(hits); i++ {
		if hits[i].pos < hits[i-1].pos+len(pairs[hits[i-1].pair].Old) {
			return "", nil, false
		}
	}
	var b strings.Builder
	spans := make([]span, 0, len(hits))
	prev := 0
	for _, h := range hits {
		p := pairs[h.pair]
		b.WriteString(text[prev:h.pos])
		start := b.Len()
		b.WriteString(p.New)
		spans = append(spans, span{start, b.Len(), h.pair})
		prev = h.pos + len(p.Old)
	}
	b.WriteString(text[prev:])
	return b.String(), spans, true
}

func replaceAt(text string, spans []span, pos int, old, repl string, pair int) (string, []span, bool) {
	end := pos + len(old)
	delta := len(repl) - len(old)
	out := make([]span, 0, len(spans)+1)
	for _, s := range spans {
		switch {
		case s.end <= pos:
			out = append(out, s)
		case s.start >= end:
			out = append(out, span{s.start + delta, s.end + delta, s.pair})
		default:
			return "", nil, false
		}
	}
	out = append(out, span{pos, pos + len(repl), pair})
	return text[:pos] + repl + text[end:], out, true
}

// parsedLines is editedLines with the comments taken from a syntax tree of the
// whole file, plus one line of context after every inserted span.
func parsedLines(c call.Change, lang language, opts Options) ([]line, bool) {
	if len(opts.Grammars) == 0 {
		return nil, false
	}
	text, spans, olds, ok := applyEdit(c, opts)
	if !ok {
		return nil, false
	}
	tree, err := opts.Grammars.Parse(c.Path, []byte(text))
	if err != nil {
		opts.note(err.Error())
	}
	if tree == nil {
		return nil, false
	}
	for _, s := range spans {
		if tree.ErrorIn(s.start, max(s.end, s.start+1)) {
			return nil, false
		}
	}
	kept := make([]map[string]int, len(olds))
	for k, old := range olds {
		kept[k] = lineCounts(old)
	}
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	var top []line
	for k := range starts {
		l := lineAt(text, starts, k, tree, lang)
		if l.code {
			break
		}
		top = append(top, l)
	}
	header := licenseHeader(top)
	lineOf := func(pos int) int { return sort.SearchInts(starts, pos+1) - 1 }
	var out []line
	for n, s := range spans {
		if n > 0 {
			out = append(out, line{})
		}
		first, last := lineOf(s.start), lineOf(max(s.end-1, s.start))
		left := maps.Clone(kept[s.pair])
		for k := first; k <= last+1 && k < len(starts); k++ {
			l := lineAt(text, starts, k, tree, lang)
			l.header = header[k]
			if c.Write {
				l.added = k <= last
				l.n = k + 1
				l.keepIfOnDisk(kept[s.pair])
			} else {
				l.added = k <= last && !take(left, strings.TrimSpace(l.text))
			}
			out = append(out, l)
		}
	}
	return out, true
}

func lineAt(text string, starts []int, k int, tree *treesitter.Tree, lang language) line {
	lo, hi := starts[k], len(text)
	if k+1 < len(starts) {
		hi = starts[k+1] - 1
	}
	raw := text[lo:hi]
	if tree.EmbeddedIn(lo, max(hi, lo+1)) {
		return classify(raw, lang)
	}
	var parts []string
	code := false
	pos := lo
	from := sort.Search(len(tree.Comments), func(i int) bool { return tree.Comments[i].End > lo })
	for _, c := range tree.Comments[from:] {
		if c.Start >= hi {
			break
		}
		a, b := max(c.Start, lo), min(c.End, hi)
		if strings.TrimSpace(text[pos:a]) != "" {
			code = true
		}
		parts = append(parts, strings.TrimSpace(text[a:b]))
		pos = b
	}
	if strings.TrimSpace(text[pos:hi]) != "" {
		code = true
	}
	comment := strings.TrimSpace(strings.Join(parts, " "))
	if comment == "" {
		return line{text: raw, code: code, inBlock: len(parts) > 0}
	}
	return line{text: raw, comment: comment, code: code}
}

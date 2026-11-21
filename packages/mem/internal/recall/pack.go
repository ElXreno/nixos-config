package recall

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mem/internal/store"
	"mem/internal/text"
)

const (
	MinTruncated = 300
	longUnit     = 240
	minPiece     = 60
	pointerLabel = 100
)

const ExpiredMark = "EXPIRED"

func Tag(r store.Record) string {
	if r.Status == store.Expired {
		return "[" + string(r.Kind) + ", " + ExpiredMark + " since " + r.ExpiredSince().Local().Format(time.DateOnly) + "]"
	}
	return "[" + string(r.Kind) + ", as of " + r.AsOf().Local().Format(time.DateOnly) + "]"
}

func Render(r store.Record, body string) string {
	var b strings.Builder
	b.WriteString("- `" + r.Key + "` " + Tag(r) + ": ")
	if r.Title != "" {
		b.WriteString(strings.TrimRight(r.Title, ".") + ". ")
	}
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			b.WriteString("\n")
			if line != "" {
				b.WriteString("  ")
			}
		}
		b.WriteString(line)
	}
	b.WriteString("\n")
	return b.String()
}

func runes(s string) int { return utf8.RuneCountInString(s) }

type unit struct {
	line int
	text string
}

func chunks(s string) []string {
	if runes(s) <= longUnit {
		return []string{s}
	}
	var out []string
	var cur []string
	n := 0
	for _, w := range atoms(s) {
		if n > 0 && n+1+runes(w) > longUnit {
			out = append(out, strings.Join(cur, " "))
			cur, n = nil, 0
		}
		if n > 0 {
			n++
		}
		cur = append(cur, w)
		n += runes(w)
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}

func sentences(line string) []string {
	if runes(line) <= longUnit {
		return []string{line}
	}
	var out []string
	start := 0
	spans := mdLink.FindAllStringIndex(line, -1)
	inLink := func(i int) bool {
		return slices.ContainsFunc(spans, func(sp []int) bool { return sp[0] < i && i < sp[1] })
	}
	for i := 0; i+1 < len(line); i++ {
		if strings.IndexByte(".;!?", line[i]) >= 0 && line[i+1] == ' ' && !inLink(i) {
			out = append(out, chunks(line[start:i+1])...)
			start = i + 2
		}
	}
	if start < len(line) {
		out = append(out, chunks(line[start:])...)
	}
	return out
}

func units(body string) []unit {
	var out []unit
	for i, line := range strings.Split(strings.TrimRight(body, " \n"), "\n") {
		for _, s := range sentences(line) {
			out = append(out, unit{line: i, text: s})
		}
	}
	return out
}

func joinUnits(us []unit, chosen map[int]string) string {
	out := ""
	prev := -1
	for i, u := range us {
		t, ok := chosen[i]
		if !ok {
			continue
		}
		marked := strings.HasPrefix(t, "...") || strings.HasSuffix(out, "...")
		switch {
		case prev < 0 && i > 0 && !marked:
			out = "... "
		case prev < 0:
		case prev != i-1:
			out = strings.TrimRight(out, " \n")
			sep, inline := "\n...\n", us[prev].line == u.line
			switch {
			case marked && inline:
				sep = " "
			case marked:
				sep = "\n"
			case inline:
				sep = " ... "
			}
			out += sep
		case us[prev].line == u.line:
			out += " "
		default:
			out += "\n"
		}
		out += t
		prev = i
	}
	return out
}

func cutSuffix(body, key string) string {
	note := "(cut here; `mem get " + key + "` prints the whole record)"
	body = strings.TrimRight(body, " \n")
	switch {
	case body == "":
		return note
	case strings.HasSuffix(body, "..."):
		return body + " " + note
	}
	return body + " ... " + note
}

var mdLink = regexp.MustCompile(`\[[^\[\]\n]*\]\([^()\s]*\)`)

func cutPoints(r []rune) []bool {
	ok := make([]bool, len(r)+1)
	ok[0], ok[len(r)] = true, true
	for i := 1; i < len(r); i++ {
		ok[i] = unicode.IsSpace(r[i-1]) || unicode.IsSpace(r[i])
	}
	s := string(r)
	for _, m := range mdLink.FindAllStringIndex(s, -1) {
		a, b := utf8.RuneCountInString(s[:m[0]]), utf8.RuneCountInString(s[:m[1]])
		for i := a + 1; i < b; i++ {
			ok[i] = false
		}
	}
	return ok
}

func atoms(s string) []string {
	r := []rune(s)
	ok := cutPoints(r)
	var out []string
	start := 0
	for i := 0; i <= len(r); i++ {
		if i == len(r) || unicode.IsSpace(r[i]) && ok[i] {
			if i > start {
				out = append(out, string(r[start:i]))
			}
			start = i + 1
		}
	}
	return out
}

func clipPrefix(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	ok := cutPoints(r)
	cut := n
	for cut > 0 && !ok[cut] {
		cut--
	}
	if cut == 0 {
		return ""
	}
	return strings.TrimRight(string(r[:cut]), " ") + " ..."
}

func snippet(line, query string, n int) string {
	flat, start, end := store.SnippetWindow(line, query, n)
	r := []rune(flat)
	if start == 0 && end == len(r) {
		return flat
	}
	ok := cutPoints(r)
	s, e := start, end
	for s < e && !ok[s] {
		s++
	}
	for e > s && !ok[e] {
		e--
	}
	if e <= s {
		return ""
	}
	out := strings.TrimSpace(string(r[s:e]))
	if s > 0 {
		out = "..." + out
	}
	if e < len(r) {
		out += "..."
	}
	return out
}

func longest(lo, hi int, ok func(int) bool) int {
	best := 0
	for lo <= hi {
		mid := (lo + hi) / 2
		if ok(mid) {
			best, lo = mid, mid+1
		} else {
			hi = mid - 1
		}
	}
	return best
}

func excerpt(r store.Record, query string, terms text.Set, share int) string {
	us := units(r.Body)
	chosen := map[int]string{}
	block := func() string { return Render(r, cutSuffix(joinUnits(us, chosen), r.Key)) }
	size := func() int { return runes(block()) }
	if size() > share {
		return ""
	}
	room := share - size()
	try := func(i int, t string, limit int) bool {
		old, had := chosen[i]
		chosen[i] = t
		if size() <= limit {
			return true
		}
		if had {
			chosen[i] = old
		} else {
			delete(chosen, i)
		}
		return false
	}
	fill := func(i int, limit int, clip func(n int) string) bool {
		if try(i, us[i].text, limit) {
			return true
		}
		fits := func(n int) bool {
			t := clip(n)
			return t != "" && try(i, t, limit)
		}
		n := longest(minPiece, runes(us[i].text), fits)
		return n > 0 && fits(n)
	}
	headLimit := share - room/2
	head := 0
	for ; head < len(us); head++ {
		if !try(head, us[head].text, headLimit) {
			break
		}
	}
	if head == 0 && len(us) > 0 {
		fill(0, headLimit, func(n int) string { return clipPrefix(us[0].text, n) })
	}
	type match struct{ i, n int }
	var ms []match
	for i := head; i < len(us); i++ {
		if _, done := chosen[i]; done {
			continue
		}
		if n := terms.Intersect(text.NewSet(us[i].text)); n > 0 {
			ms = append(ms, match{i, n})
		}
	}
	sort.SliceStable(ms, func(a, b int) bool { return ms[a].n > ms[b].n })
	for _, m := range ms {
		line := us[m.i].text
		fill(m.i, share, func(n int) string { return snippet(line, query, n) })
	}
	for i := head; i < len(us); i++ {
		if _, done := chosen[i]; done {
			continue
		}
		if !fill(i, share, func(n int) string { return clipPrefix(us[i].text, n) }) {
			break
		}
	}
	return block()
}

const PointerHeader = "Agent memory: possibly related, not loaded (`mem get KEY` prints one).\n"

func RenderPointer(r store.Record) string {
	line := "- `" + r.Key + "` " + Tag(r)
	label := strings.TrimRight(r.Title, ".")
	if label == "" {
		for _, l := range strings.Split(r.Body, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				label = clipPrefix(l, pointerLabel)
				break
			}
		}
	}
	if label != "" {
		line += ": " + label
	}
	if r.Status == store.Expired {
		line += " (unverified: check it, then `mem verify` or `mem retire`)"
	}
	return line + "\n"
}

func PackPointers(recs []store.Record, budget int) Packed {
	var p Packed
	var b strings.Builder
	used := runes(PointerHeader)
	for i, r := range recs {
		line := RenderPointer(r)
		if used+runes(line)-1 > budget {
			for _, x := range recs[i:] {
				p.Dropped = append(p.Dropped, x.Key)
			}
			break
		}
		b.WriteString(line)
		used += runes(line)
		p.Keys = append(p.Keys, r.Key)
	}
	if len(p.Keys) > 0 {
		p.Text = strings.TrimRight(PointerHeader+b.String(), "\n")
	}
	return p
}

type Packed struct {
	Text    string
	Keys    []string
	Cut     []string
	Dropped []string
}

func fit(recs []store.Record, query string, terms text.Set, avail int) ([]string, []bool, bool) {
	n := len(recs)
	blocks, cut := make([]string, n), make([]bool, n)
	full := make([]string, n)
	for i, r := range recs {
		full[i] = Render(r, r.Body)
	}
	remaining, open := avail, n
	for changed := true; changed && open > 0; {
		changed = false
		share := remaining / open
		for i := range recs {
			if blocks[i] == "" && runes(full[i]) <= share {
				blocks[i] = full[i]
				remaining -= runes(full[i])
				open--
				changed = true
			}
		}
	}
	for i, r := range recs {
		if blocks[i] != "" {
			continue
		}
		share := remaining / open
		if share < MinTruncated {
			return nil, nil, false
		}
		b := excerpt(r, query, terms, share)
		if b == "" {
			return nil, nil, false
		}
		blocks[i], cut[i] = b, true
		remaining -= runes(b)
		open--
	}
	return blocks, cut, true
}

func Pack(header string, recs []store.Record, query string, budget int) Packed {
	avail := budget - runes(header)
	terms := text.NewSet(query)
	for n := len(recs); n > 0; n-- {
		blocks, cut, ok := fit(recs[:n], query, terms, avail)
		if !ok {
			continue
		}
		var b strings.Builder
		b.WriteString(header)
		var p Packed
		for i, r := range recs[:n] {
			b.WriteString(blocks[i])
			p.Keys = append(p.Keys, r.Key)
			if cut[i] {
				p.Cut = append(p.Cut, r.Key)
			}
		}
		for _, r := range recs[n:] {
			p.Dropped = append(p.Dropped, r.Key)
		}
		p.Text = strings.TrimRight(b.String(), "\n")
		return p
	}
	var p Packed
	for _, r := range recs {
		p.Dropped = append(p.Dropped, r.Key)
	}
	return p
}

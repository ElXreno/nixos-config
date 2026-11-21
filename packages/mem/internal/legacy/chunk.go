package legacy

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	dateLedRe = regexp.MustCompile(`^[-*+>\s#]*\**\[?20\d\d-\d\d-\d\d`)
	headingRe = regexp.MustCompile(`^#{1,6}\s+\S`)
	fenceRe   = regexp.MustCompile("^\\s*(```|~~~)")
)

func journalLines(body string) int {
	n := 0
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if !inFence && dateLedRe.MatchString(line) {
			n++
		}
	}
	return n
}

type chunk struct {
	heading string
	text    string
}

type block struct {
	heading string
	lines   []string
	starts  bool
}

func runes(s string) int { return utf8.RuneCountInString(s) }

func headingText(line string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
}

func blocks(body string) []block {
	var out []block
	var cur *block
	heading := ""
	inFence := false
	prevBlank := true
	for _, line := range strings.Split(body, "\n") {
		blank := strings.TrimSpace(line) == ""
		fence := fenceRe.MatchString(line)
		isHeading := !inFence && headingRe.MatchString(line)
		start := !inFence && !blank && (isHeading || prevBlank || dateLedRe.MatchString(line))
		if isHeading {
			heading = headingText(line)
		}
		if start || cur == nil {
			out = append(out, block{heading: heading, starts: isHeading})
			cur = &out[len(out)-1]
		}
		cur.lines = append(cur.lines, line)
		if fence {
			inFence = !inFence
		}
		prevBlank = blank && !inFence
	}
	return out
}

func splitLong(s string, limit int) []string {
	var out []string
	for runes(s) > limit {
		r := []rune(s)
		cut := limit
		for i := limit; i > limit/2; i-- {
			if r[i] == ' ' || r[i] == '\n' {
				cut = i
				break
			}
		}
		out = append(out, strings.TrimSpace(string(r[:cut])))
		s = strings.TrimSpace(string(r[cut:]))
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

func chunkBody(body string, target int) []chunk {
	if target <= 0 {
		target = DefaultChunkRunes
	}
	hard := target * 3 / 2
	var out []chunk
	var cur []string
	curLen := 0
	curHeading := ""
	flush := func() {
		text := strings.TrimSpace(strings.Join(cur, "\n"))
		if text != "" {
			out = append(out, chunk{heading: curHeading, text: text})
		}
		cur, curLen = nil, 0
	}
	add := func(piece, heading string) {
		if curLen == 0 {
			curHeading = heading
		}
		cur = append(cur, piece)
		curLen += runes(piece) + 1
	}
	for _, b := range blocks(body) {
		text := strings.TrimRight(strings.Join(b.lines, "\n"), "\n ")
		n := runes(text)
		if curLen > 0 && (curLen+n > target || (b.starts && curLen >= target/3)) {
			flush()
		}
		if n <= hard {
			add(text, b.heading)
			continue
		}
		for _, line := range b.lines {
			for _, piece := range splitLong(line, hard) {
				if curLen > 0 && curLen+runes(piece) > target {
					flush()
				}
				add(piece, b.heading)
			}
		}
	}
	flush()
	return out
}

// Package edit holds the checks that run on file edits and writes.
package edit

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"agent-guard/internal/call"
	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

// markers are code points: Go rejects a literal byte order mark in source.
var markers = []struct {
	codepoints []rune
	what       string
}{
	{[]rune{0x2014}, "an em-dash"},
	{[]rune{0x2013}, "an en-dash"},
	{[]rune{0x2026}, "an ellipsis character instead of three dots"},
	{[]rune{0x201C, 0x201D}, "curly double quotes"},
	{[]rune{0x2018, 0x2019}, "a curly apostrophe or single quote"},
	{[]rune{0x00A0, 0x202F}, "a non-breaking space"},
	{[]rune{0x200B, 0x200C, 0x200D, 0xFEFF}, "an invisible zero-width character"},
}

const maxGlyphLiteral = 16

var (
	literalPathRe = regexp.MustCompile(`(?i)(\.(rpy|po|pot|srt|vtt)$|(^|/)(trim|strip)[^/]*\.pl$)`)
	testPathRe    = regexp.MustCompile(`(_test\.go|_test\.py|(^|/)test_[^/]*\.py)$|(^|/)tests?/`)
	proseExtRe    = regexp.MustCompile(`(?i)\.(md|markdown|txt|rst|adoc|org)$`)
	substituteRe  = regexp.MustCompile(`(^\s*|['"]|=~\s*)(s|tr|y)([/|#!])`)
	commentLeadRe = regexp.MustCompile(`^\s*(#|//|--|/\*|\*)`)
)

func markerIn(s string) (string, bool) {
	for _, m := range markers {
		for _, r := range m.codepoints {
			if strings.ContainsRune(s, r) {
				return m.what, true
			}
		}
	}
	return "", false
}

// markerOutsideLiteral skips glyphs that are data in code: classes, substitutions, test strings.
func markerOutsideLiteral(s string, code, test bool) (string, bool) {
	var strs [][2]int
	if code && !commentLeadRe.MatchString(s) {
		strs = quoted(s)
	}
	for i, r := range s {
		what, ok := markerIn(string(r))
		if !ok || literalAt(s, i, r, strs, code, test) {
			continue
		}
		return what, true
	}
	return "", false
}

// quoted pairs the quotes of a code line left to right, up to a trailing comment.
func quoted(s string) [][2]int {
	var out [][2]int
	open, q := 0, byte(0)
	for k := 0; k < len(s); k++ {
		c := s[k]
		switch {
		case q != 0:
			if c == q && s[k-1] != '\\' {
				out = append(out, [2]int{open, k})
				q = 0
			}
		case c == '"' || (c == '\'' && (k == 0 || !wordByte(s[k-1]))):
			open, q = k, c
		case c == '#' || (c == '/' && strings.HasPrefix(s[k:], "//")):
			if k == 0 || s[k-1] == ' ' || s[k-1] == '\t' {
				return out
			}
		}
	}
	return out
}

func wordByte(c byte) bool {
	return c >= 0x80 || c == '_' || (c >= '0' && c <= '9') || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

func pictographic(r rune) bool {
	return (r >= 0x2600 && r <= 0x27BF) || (r >= 0x1F000 && r <= 0x1FAFF) || r == 0xFE0F
}

func literalAt(s string, i int, r rune, strs [][2]int, code, test bool) bool {
	end := i + utf8.RuneLen(r)
	if r == 0x200D {
		prev, _ := utf8.DecodeLastRuneInString(s[:i])
		next, _ := utf8.DecodeRuneInString(s[end:])
		if pictographic(prev) && pictographic(next) {
			return true
		}
	}
	if !code {
		return false
	}
	if open := strings.LastIndexAny(s[:i], "[] \t"); open >= 0 && s[open] == '[' {
		if close := strings.IndexAny(s[end:], "[] \t"); close >= 0 && s[end+close] == ']' {
			class := s[open+1 : end+close]
			quotedClass := open > 0 && strings.ContainsRune("\"'/`", rune(s[open-1]))
			if quotedClass || strings.HasPrefix(class, "^") || glyphs(class) > 1 {
				return true
			}
		}
	}
	for _, m := range substituteRe.FindAllStringSubmatchIndex(s, -1) {
		delim, from := s[m[6]:m[7]], m[7]
		second := strings.Index(s[from:], delim)
		if second < 0 {
			continue
		}
		third := strings.Index(s[from+second+1:], delim)
		if third < 0 {
			continue
		}
		if from <= i && end <= from+second+1+third {
			return true
		}
	}
	for _, sp := range strs {
		if sp[0] >= i || end > sp[1] {
			continue
		}
		inner := s[sp[0]+1 : sp[1]]
		if utf8.RuneCountInString(inner) <= maxGlyphLiteral && !strings.ContainsFunc(inner, func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }) {
			return true
		}
		if test && s[sp[0]] == '"' && !strings.Contains(s, `"""`) {
			return true
		}
	}
	return false
}

func glyphs(s string) int {
	n := 0
	for _, r := range s {
		if _, ok := markerIn(string(r)); ok {
			n++
		}
	}
	return n
}

// AIMarkers warns on typography that marks machine-written text.
func AIMarkers(c call.Change, opts Options) verdict.Verdicts {
	slash := filepath.ToSlash(c.Path)
	if opts.memory(c.Path) || literalPathRe.MatchString(slash) {
		return nil
	}
	code := !proseExtRe.MatchString(slash)
	test := code && testPathRe.MatchString(slash)
	if c.Write {
		return markersAdded(c.Content, "", code, test)
	}
	for _, p := range c.Pairs {
		if v := markersAdded(p.New, p.Old, code, test); v != nil {
			return v
		}
	}
	return nil
}

func markersAdded(added, old string, code, test bool) verdict.Verdicts {
	if _, ok := markerIn(added); !ok {
		return nil
	}
	kept := map[string]bool{}
	for _, line := range strings.Split(old, "\n") {
		if line != "" {
			kept[line] = true
		}
	}
	for _, line := range strings.Split(added, "\n") {
		what, ok := markerOutsideLiteral(line, code, test)
		if !ok || kept[line] {
			continue
		}
		return verdict.Verdicts{{Level: verdict.Advise, Check: "ai-markers",
			Message: "This writes " + what + " to disk: \"" + text.Clip(line, 70) + "\". Those are AI markers the user strips by hand; use a plain hyphen, straight quotes, three dots or an ordinary space instead. Arrows and other technical glyphs are fine."}}
	}
	return nil
}

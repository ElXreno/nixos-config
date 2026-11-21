package ui

import (
	"regexp"
	"strings"
)

var journalRe = regexp.MustCompile(`(?m)^[ \t]*(?:[-*][ \t]+)?(?:\*\*)?(?:Narrowed |Corrected |Updated )?20\d\d-\d\d-\d\d`)

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func firstLine(body string) string {
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#-*> "))
		if l != "" {
			return truncRunes(l, 180)
		}
	}
	return ""
}

package edit

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

const maxCited = 3

var (
	storyRe       = regexp.MustCompile(`(?i)\b(used to be|renamed (from|to)|temporarily (disabled|commented)|commented out for now)\b|^[\s/#*;!-]*regression:`)
	dateRe        = regexp.MustCompile(`(^|[^\w./-])(19|20)\d\d-(0[1-9]|1[0-2])(-(0[1-9]|[12]\d|3[01]))?($|[^\w/-])|(?i:\b(january|february|march|april|june|july|august|september|october|november|december|jan|feb|apr|jun|jul|aug|sept?|oct|nov)\.?\s+(19[5-9]\d|20[0-4]\d)\b)|\b(Mar|May|Dec)\.?\s+(19[5-9]\d|20[0-4]\d)\b|(?i:(^|[^\p{L}])((январ|феврал|март|апрел|июн|июл|август|сентябр|октябр|ноябр|декабр)\p{L}*|ма[йя])\s+(19[5-9]\d|20[0-4]\d))`)
	urlRe         = regexp.MustCompile(`[a-z][a-z0-9+.-]*://`)
	backtickRe    = regexp.MustCompile("`[^`]*`")
	storyExemptRe = regexp.MustCompile(`(?i)(^|/)(migrat(e|es|ion|ions)?|compat|changelog)([/._-]|$)`)
)

func plainText(c string) string {
	if urlRe.MatchString(c) || licenseAnchorRe.MatchString(c) {
		return ""
	}
	return backtickRe.ReplaceAllString(c, "")
}

func storyIn(c string) string {
	return storyRe.FindString(plainText(c))
}

func dateIn(c string) bool {
	return dateRe.MatchString(plainText(c))
}

// contentAdvice flags added comment lines that narrate history or carry a date.
func contentAdvice(path string, lines []line) verdict.Verdicts {
	storyAllowed := !storyExemptRe.MatchString(filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path))
	var story, dated []string
	matched := ""
	for _, l := range lines {
		if l.comment == "" || !l.added || l.header || directiveRe.MatchString(l.comment) {
			continue
		}
		if m := storyIn(l.comment); m != "" && storyAllowed {
			if matched == "" {
				matched = strings.TrimSpace(strings.ToLower(m))
			}
			story = append(story, cite(l))
		}
		if dateIn(l.comment) {
			dated = append(dated, cite(l))
		}
	}
	var out verdict.Verdicts
	if len(story) > 0 {
		out = append(out, verdict.Verdict{Level: verdict.Advise, Check: "comment-story",
			Message: "These comment lines narrate history or status (matched \"" + matched + "\"): " + citations(story) +
				". A file describes the code as it is now; how it got there belongs in the commit message or the project memory."})
	}
	if len(dated) > 0 {
		out = append(out, verdict.Verdict{Level: verdict.Advise, Check: "comment-date",
			Message: "These comment lines carry a date: " + citations(dated) +
				". A date in a file ages with it; it belongs in the commit message, an issue or the project memory."})
	}
	return out
}

func cite(l line) string {
	s := "\"" + text.Clip(l.comment, 100) + "\""
	if l.n > 0 {
		s = "L" + strconv.Itoa(l.n) + " " + s
	}
	return s
}

func citations(c []string) string {
	if len(c) > maxCited {
		return strings.Join(c[:maxCited], "; ") + "; +" + strconv.Itoa(len(c)-maxCited) + " more"
	}
	return strings.Join(c, "; ")
}

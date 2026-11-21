package edit

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"agent-guard/internal/call"
	"agent-guard/internal/lang"
	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

const (
	maxCommentLine   = 100
	softLineLimit    = 120
	maxSoftLines     = 3
	configBlockLimit = 1
	codeBlockLimit   = 2
	docBlockLimit    = 6
	budgetFloor      = 3
	licenseWrap      = 3
	maxNoticeLine    = 140
	maxListed        = 8
)

var (
	directiveRe     = regexp.MustCompile(`^(#!|#\s*(shellcheck|noqa|type:|pyright:|ruff:|fmt:|pragma|-\*-|yaml-language-server|\{"\$imagepolicy"|renovate|checkov|kics|tflint|trivy|nixpkgs|SPDX|Copyright|region|endregion)|//\s*(go:|nolint|\+build|SPDX|eslint|@ts-|prettier-ignore|biome-ignore)|//go:|--\s*(SPDX|sqlfluff|noqa))`)
	commentedCodeRe = regexp.MustCompile(`^(#|//|--)\s*(-\s*)?[A-Za-z_"'\[][A-Za-z0-9_.\-\[\]"']*\s*[:=]\s*(\S+(\s+\S+){0,2})?\s*$`)
	declarationRe   = regexp.MustCompile(`^\s*(#\[|@|pub(\([a-z]+\))?\s+|(async\s+)?(func|fn|type|var|const|let|package|def|class|struct|enum|impl|interface|trait|mod|export|function|static|final|public|private|protected|module|proc|macro_rules!)\b)`)
	innerDocRe      = regexp.MustCompile(`^(//!|///|/\*\*)`)
	cgoImportRe     = regexp.MustCompile(`^\s*import\s+"C"\s*$`)
	licenseAnchorRe = regexp.MustCompile(`(?i)(spdx-license-identifier|copyright|\(c\)\s*\d{4}|\x{00A9}|licensed under|permission is hereby granted|general public license|apache license|mozilla public|redistribution and use|cddl header|common development and distribution|all rights reserved|public domain|\bcc0\b|unlicense)`)
	licenseWordRe   = regexp.MustCompile(`(?i)(licen[cs]|copyright|spdx|warrant(y|ies)|redistribut|distributed (under|in the hope|with|on an)|distribute(,| it| this| the software| copies)|with the distribution|permission (is|to|notice)|written permission|permitted provided|free software|general public|\bliab|merchantab|"as is"|as-is|disclaim|damages|such damage|infring|\btort|dealings|cddl|apache license|mozilla public|\bmpl\b|\bmit\b|\bbsd\b|rights reserved|\(c\)|\x{00A9}|endorse or promote|covered code|public domain|unencumbered|royalt|\bcc0\b|patent|intellectual property)`)
	noticeRe        = regexp.MustCompile(`(?i)^[\s*/#;!-]*(spdx-|copyright\s*(\(c\)|\x{00A9}|\d{4}))`)
)

type language struct {
	prefixes       []string
	config         bool
	doc            bool
	cBlocks        bool
	hashNeedsSpace bool
	cgo            bool
}

func languageOf(path string) (language, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch lang.Of(path).Class {
	case lang.Config:
		return language{prefixes: []string{"#", "//"}, config: true}, true
	case lang.Hash:
		return language{prefixes: []string{"#"}, doc: true, hashNeedsSpace: ext == ".nix"}, true
	case lang.Slash:
		cFamily := ext == ".c" || ext == ".h" || ext == ".cpp" || ext == ".hpp" || ext == ".cc"
		return language{prefixes: []string{"//", "/*", "*", "*/"}, doc: true, cBlocks: cFamily, cgo: ext == ".go"}, true
	case lang.Dash:
		return language{prefixes: []string{"--"}, doc: true}, true
	}
	return language{}, false
}

type line struct {
	text    string
	comment string
	code    bool
	added   bool
	header  bool
	inBlock bool
	n       int
}

func classify(raw string, lang language) line {
	t := strings.TrimSpace(raw)
	if t == "" {
		return line{text: raw}
	}
	for _, p := range lang.prefixes {
		if !strings.HasPrefix(t, p) {
			continue
		}
		if p == "*" && t != "*" && !strings.HasPrefix(t, "* ") && !strings.HasPrefix(t, "*/") {
			continue
		}
		if p == "#" && lang.hashNeedsSpace && !opensComment(t[1:]) {
			continue
		}
		return line{text: raw, comment: t}
	}
	if i := inlineComment(t, lang); i >= 0 {
		return line{text: raw, comment: strings.TrimSpace(t[i:]), code: true}
	}
	return line{text: raw, code: true}
}

// inlineComment finds a trailing comment outside quotes, or -1.
func inlineComment(t string, lang language) int {
	inS, inD := false, false
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		}
		if inS || inD || i == 0 || (t[i-1] != ' ' && t[i-1] != '\t') {
			continue
		}
		for _, p := range lang.prefixes {
			if p == "*" || p == "*/" {
				continue
			}
			if p == "#" && lang.hashNeedsSpace && !opensComment(t[i+1:]) {
				continue
			}
			if strings.HasPrefix(t[i:], p) {
				return i
			}
		}
	}
	return -1
}

func opensComment(rest string) bool {
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '#'
}

func exempt(c string) bool {
	return directiveRe.MatchString(c) || commentedCodeRe.MatchString(c)
}

func bare(c string) bool {
	return strings.Trim(c, "/*#!-;=[]{} \t") == ""
}

func licenseWording(c string) bool {
	return !bare(c) && licenseWordRe.MatchString(c)
}

// licenseHeader marks the leading comment blocks that are mostly license text.
func licenseHeader(top []line) map[int]bool {
	out := map[int]bool{}
	gap := func(l line) bool { return l.comment == "" && !l.inBlock }
	for i := 0; i < len(top); {
		if gap(top[i]) {
			i++
			continue
		}
		j := i
		for j < len(top) && !gap(top[j]) {
			j++
		}
		markLicense(top[i:j], i, out)
		i = j
	}
	return out
}

func markLicense(block []line, offset int, out map[int]bool) {
	anchored, words, text := false, 0, 0
	for _, l := range block {
		if bare(l.comment) {
			continue
		}
		text++
		if licenseWordRe.MatchString(l.comment) {
			words++
		}
		anchored = anchored || licenseAnchorRe.MatchString(l.comment)
	}
	if !anchored || words*2 < text {
		return
	}
	seen := false
	for i := 0; i < len(block); {
		if bare(block[i].comment) || licenseWording(block[i].comment) {
			out[offset+i] = true
			seen = seen || licenseWording(block[i].comment)
			i++
			continue
		}
		j, prose := i, 0
		for j < len(block) && !licenseWording(block[j].comment) {
			if !bare(block[j].comment) {
				prose++
			}
			j++
		}
		wrap := seen && j < len(block) && prose <= licenseWrap
		for k := i; k < j; k++ {
			if wrap || bare(block[k].comment) {
				out[offset+k] = true
			}
		}
		i = j
	}
}

func markHeader(lines []line) {
	end := 0
	for end < len(lines) && !lines[end].code {
		end++
	}
	for i := range licenseHeader(lines[:end]) {
		lines[i].header = true
	}
}

func (l *line) keepIfOnDisk(onDisk map[string]int) {
	key := strings.TrimSpace(l.text)
	if l.comment == "" || (!l.code && bare(l.comment)) || onDisk[key] == 0 {
		return
	}
	onDisk[key]--
	if l.code {
		l.comment = ""
		return
	}
	l.added = false
}

func lineCounts(text string) map[string]int {
	counts := map[string]int{}
	for raw := range strings.SplitSeq(text, "\n") {
		counts[strings.TrimSpace(raw)]++
	}
	return counts
}

// Comments reviews the lines an edit adds and rejects comment walls.
func Comments(c call.Change, opts Options) verdict.Verdicts {
	lang, ok := languageOf(c.Path)
	if !ok || opts.memory(c.Path) {
		return nil
	}
	lines, ok := parsedLines(c, lang, opts)
	if !ok {
		lines = editedLines(c, lang, opts)
	}
	blockLimit := codeBlockLimit
	if lang.config {
		blockLimit = configBlockLimit
	}
	codeCount, commentCount := 0, 0
	type problem struct{ ref, text string }
	var problems []problem
	hard := false
	note := func(from, to int, what, sample string) {
		ref := ""
		if from > 0 {
			ref = "L" + strconv.Itoa(from)
			if to > from {
				ref += "-L" + strconv.Itoa(to)
			}
		}
		problems = append(problems, problem{ref, what + ": \"" + text.Clip(sample, 70) + "\""})
	}
	long := func(n int, c string) {
		size := utf8.RuneCountInString(c)
		hard = hard || size > softLineLimit || storyIn(c) != "" || dateIn(c)
		note(n, 0, "comment line of "+strconv.Itoa(size)+" characters (limit "+strconv.Itoa(maxCommentLine)+")", c)
	}

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if l.code && l.added {
			codeCount++
		}
		if l.comment == "" || (l.code && !l.added) {
			continue
		}
		if l.code {
			if !exempt(l.comment) {
				commentCount++
				if utf8.RuneCountInString(l.comment) > maxCommentLine {
					long(l.n, l.comment)
				}
			}
			continue
		}
		// Measure the block of consecutive comment-only lines starting here; only
		// the lines this edit adds count, the rest is context.
		j := i
		prose, from, to := 0, 0, 0
		first, head := "", ""
		for j < len(lines) && lines[j].comment != "" && !lines[j].code {
			lj := lines[j]
			c := lj.comment
			if head == "" && !directiveRe.MatchString(c) && (!lj.header || bare(c)) {
				head = c
			}
			if lj.added && !exempt(c) {
				size := utf8.RuneCountInString(c)
				if size > maxCommentLine && (size > maxNoticeLine || !lj.header || !noticeRe.MatchString(c)) {
					long(lj.n, c)
				}
				if !lj.header {
					prose++
					if first == "" {
						first, from = c, lj.n
					}
					to = lj.n
				}
			}
			j++
		}
		i = j - 1
		if prose == 0 || (lang.cgo && j < len(lines) && cgoImportRe.MatchString(lines[j].text)) {
			continue
		}
		cBlock := lang.cBlocks && (strings.HasPrefix(head, "/*") || strings.HasPrefix(head, "*"))
		doc := lang.doc &&
			((j < len(lines) && declarationRe.MatchString(lines[j].text)) ||
				innerDocRe.MatchString(head) || cBlock)
		if doc && (cBlock || prose <= docBlockLimit) {
			continue
		}
		if prose > blockLimit {
			hard = true
			note(from, to, strconv.Itoa(prose)+" consecutive comment lines", first)
		}
		commentCount += prose
	}
	advice := contentAdvice(c.Path, lines)
	var listed []string
	for k, p := range problems {
		if k == maxListed {
			var refs []string
			for _, rest := range problems[k:] {
				if rest.ref != "" {
					refs = append(refs, rest.ref)
				}
			}
			more := "+" + strconv.Itoa(len(problems)-k) + " more"
			if len(refs) > 0 {
				more += ": " + strings.Join(refs, ", ")
			}
			listed = append(listed, more)
			break
		}
		if p.ref != "" {
			p.text = p.ref + " " + p.text
		}
		listed = append(listed, p.text)
	}
	if commentCount >= budgetFloor && commentCount > codeCount {
		hard = true
		listed = append(listed, strconv.Itoa(commentCount)+" comment lines for "+strconv.Itoa(codeCount)+" lines of code")
	}
	if len(listed) == 0 {
		return advice
	}
	if !hard && c.Write && !lang.config && len(problems) <= maxSoftLines {
		return append(verdict.Verdicts{{Level: verdict.Advise, Check: "comments", Findings: listed,
			Message: "This " + opts.Vocab.WriteTool + " goes through with long comment lines (" + strings.Join(listed, "; ") +
				"). A comment is one short line that points at the value beside it, like `# workaround for <url>`, never a story; an " + opts.Vocab.EditTool + " that shortens these lines is enough, nothing else in the file tripped this check."}}, advice...)
	}
	if underDir(c.Path, opts.Scratch) {
		return append(verdict.Verdicts{{Level: verdict.Advise, Check: "comments", Findings: listed,
			Message: "This edit goes through because the file is under the scratch directory, with comment problems (" + strings.Join(listed, "; ") +
				"). A comment is one short line that points at the value beside it, like `# workaround for <url>`, never a story."}}, advice...)
	}
	where := "source files"
	if lang.config {
		where = "config files"
	}
	fix := ""
	if len(problems) > 0 {
		fix = "Fix every listed line in one pass, cutting or shortening it rather than rewording it into more comment lines; no other line in the edit tripped this check. "
	}
	return append(verdict.Verdicts{{Level: verdict.Deny, Check: "comments", Findings: listed,
		Message: "Blocked: comment wall in " + where + " (" + strings.Join(listed, "; ") +
			"). A comment is one short line that points at the value beside it, like `# workaround for <url>` or `# enabled for <thing>`. " +
			"What happened, when, why it was measured that way and what was decided belongs in the project memory or the conversation, not in the file. " +
			fix + "A leading license header is exempt. Keep the values, drop the story, and re-apply the edit."}}, advice...)
}

// editedLines marks the lines a change introduces, without a syntax tree.
func editedLines(c call.Change, lang language, opts Options) []line {
	var out []line
	if c.Write {
		onDisk := lineCounts(opts.previous(c.Path))
		n := 0
		for raw := range strings.SplitSeq(c.Content, "\n") {
			n++
			l := classify(raw, lang)
			l.added = true
			l.n = n
			l.keepIfOnDisk(onDisk)
			out = append(out, l)
		}
		markHeader(out)
		return out
	}
	for _, p := range c.Pairs {
		kept := lineCounts(p.Old)
		if len(out) > 0 {
			out = append(out, line{})
		}
		for raw := range strings.SplitSeq(p.New, "\n") {
			l := classify(raw, lang)
			l.added = !take(kept, strings.TrimSpace(raw))
			out = append(out, l)
		}
	}
	return out
}

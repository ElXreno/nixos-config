package legacy

import (
	"regexp"
	"slices"
	"strings"
)

var (
	linkRe       = regexp.MustCompile(`\[\[([^\[\]\n]+)\]\]`)
	inlineCodeRe = regexp.MustCompile("`[^`\n]*`")
	normSepRe    = regexp.MustCompile(`[\s_-]+`)
	slugPrefix   = []string{"feedback_", "reference_", "project_", "user_"}
)

func stripCode(s string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if !inFence {
			b.WriteString(inlineCodeRe.ReplaceAllString(line, " "))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func linkTargets(s string) []string {
	var out []string
	for _, m := range linkRe.FindAllStringSubmatch(stripCode(s), -1) {
		t := cleanTarget(m[1])
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func cleanTarget(raw string) string {
	t, _, _ := strings.Cut(raw, "|")
	t, _, _ = strings.Cut(t, "#")
	t = strings.TrimSpace(t)
	t = strings.TrimSuffix(t, ".md")
	return strings.TrimSpace(t)
}

func normName(s string) string {
	return strings.Trim(normSepRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_"), "_")
}

type resolver struct {
	slugs map[string]bool
	names map[string][]string
	norms map[string][]string
}

func newResolver() *resolver {
	return &resolver{slugs: map[string]bool{}, names: map[string][]string{}, norms: map[string][]string{}}
}

func addUnique(m map[string][]string, k, v string) {
	if k != "" && !slices.Contains(m[k], v) {
		m[k] = append(m[k], v)
	}
}

func (r *resolver) add(slug, name string) {
	r.slugs[slug] = true
	addUnique(r.names, strings.ToLower(strings.TrimSpace(name)), slug)
	addUnique(r.norms, normName(slug), slug)
}

func (r *resolver) resolve(target string) (string, bool) {
	if r.slugs[target] {
		return target, true
	}
	if c := r.names[strings.ToLower(target)]; len(c) == 1 {
		return c[0], true
	}
	n := normName(target)
	if c := r.norms[n]; len(c) == 1 {
		return c[0], true
	}
	var found []string
	for _, p := range slugPrefix {
		for _, s := range r.norms[normName(p+n)] {
			if !slices.Contains(found, s) {
				found = append(found, s)
			}
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return "", false
}

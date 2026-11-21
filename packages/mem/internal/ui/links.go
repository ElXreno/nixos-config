package ui

import (
	"regexp"
	"sort"
	"strings"

	"mem/internal/store"
)

const (
	resolveExact   = "exact"
	resolveDerived = "derived"
	resolvePrefix  = "prefix"
	maxResolved    = 12
	minPrefix      = 4
)

var keyLike = regexp.MustCompile(`^[a-z0-9][a-z0-9_.#-]*$`)

type linkResolver struct {
	keys    map[string]bool
	derived map[string][]string
	sorted  []string
}

func newLinkResolver(groups []*group, links []rawLink) *linkResolver {
	lr := &linkResolver{keys: make(map[string]bool, len(groups)), derived: map[string][]string{}}
	baseOf := map[string]string{}
	for _, g := range groups {
		lr.keys[g.base] = true
		lr.sorted = append(lr.sorted, g.base)
		for _, r := range g.recs {
			baseOf[r.Key] = g.base
		}
	}
	sort.Strings(lr.sorted)
	seen := map[[2]string]bool{}
	for _, l := range links {
		if l.rel != string(store.DerivedFrom) {
			continue
		}
		src, ok := baseOf[l.src]
		if !ok {
			continue
		}
		slug, _ := BaseKey(l.dst)
		if lr.keys[slug] || seen[[2]string{slug, src}] {
			continue
		}
		seen[[2]string{slug, src}] = true
		lr.derived[slug] = append(lr.derived[slug], src)
	}
	for k := range lr.derived {
		sort.Strings(lr.derived[k])
	}
	return lr
}

func (lr *linkResolver) valid(token string) bool {
	return keyLike.MatchString(token) && len(token) <= store.MaxKeyLen
}

func (lr *linkResolver) resolve(token string) ([]string, string) {
	if !lr.valid(token) {
		return nil, ""
	}
	forms := []string{token}
	if norm := strings.ReplaceAll(token, "-", "_"); norm != token {
		forms = append(forms, norm)
	}
	for _, f := range forms {
		base, _ := BaseKey(f)
		if lr.keys[base] {
			return []string{base}, resolveExact
		}
	}
	for _, f := range forms {
		if srcs := lr.derived[f]; len(srcs) > 0 {
			return capKeys(srcs), resolveDerived
		}
	}
	for _, f := range forms {
		f = strings.TrimSuffix(f, "_hub")
		if len(f) < minPrefix {
			continue
		}
		f += "_"
		i := sort.SearchStrings(lr.sorted, f)
		var out []string
		for ; i < len(lr.sorted) && strings.HasPrefix(lr.sorted[i], f); i++ {
			out = append(out, lr.sorted[i])
		}
		if len(out) > 0 {
			return capKeys(out), resolvePrefix
		}
	}
	return nil, ""
}

func capKeys(keys []string) []string {
	if len(keys) > maxResolved {
		keys = keys[:maxResolved]
	}
	return append([]string{}, keys...)
}

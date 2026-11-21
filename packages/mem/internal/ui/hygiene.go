package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"mem/internal/store"
)

const (
	surfacedDays    = 30
	journalMin      = 3
	reasonNoTopic   = "no-topic"
	reasonNoVector  = "no-vector"
	reasonStaleVec  = "stale-vector"
	reasonUnsurface = "unsurfaced"
	scopeKind       = "scope"
)

var ownerRepo = regexp.MustCompile(`^[a-z0-9_.+@-]+/[a-z0-9_.+@-]+$`)

type HygieneItem struct {
	Key    string   `json:"k"`
	Detail string   `json:"d,omitempty"`
	Why    []string `json:"w,omitempty"`
	Kind   string   `json:"t,omitempty"`
}

type HygieneSection struct {
	ID      string        `json:"id"`
	Counted bool          `json:"counted"`
	Items   []HygieneItem `json:"items"`
}

type ImportRow struct {
	Project   string `json:"project"`
	Records   int    `json:"records"`
	Logs      int    `json:"logs"`
	Untopiced int    `json:"untopiced"`
}

type Surfaced struct {
	WindowDays  int  `json:"window_days"`
	CoveredDays int  `json:"covered_days"`
	Applied     bool `json:"applied"`
}

type Hygiene struct {
	Sections []HygieneSection `json:"sections"`
	Count    int              `json:"count"`
	Imports  []ImportRow      `json:"imports"`
	Surfaced Surfaced         `json:"surfaced"`
	Dropped  int              `json:"dropped"`
}

type baseHygiene struct {
	sections map[string][]HygieneItem
	imports  map[string]*ImportRow
	scopes   map[string]int
	dropped  int
}

var sectionOrder = []struct {
	id      string
	counted bool
}{
	{"broken", true}, {"orphans", true}, {"expired", true}, {"stale", true}, {"dangling", true}, {"triggers", true},
	{"verify", false}, {"scopes", false}, {"duplicates", false}, {"journals", false}, {"superseded", false}, {"retired", false},
}

func newBaseHygiene() baseHygiene {
	return baseHygiene{sections: map[string][]HygieneItem{}, imports: map[string]*ImportRow{}, scopes: map[string]int{}}
}

func (b *baseHygiene) add(section string, it HygieneItem) {
	b.sections[section] = append(b.sections[section], it)
}

func projectsOf(scopes []string) []string {
	var out []string
	for _, s := range scopes {
		if p, ok := strings.CutPrefix(s, "project:"); ok {
			out = append(out, p)
		}
	}
	return out
}

func (b *baseHygiene) collect(gr *group, n *Node, topic string, baseOf map[string]string, now time.Time) {
	first := gr.recs[0]
	status := groupStatus(gr)
	for _, p := range projectsOf(n.Scopes) {
		row := b.imports[p]
		if row == nil {
			row = &ImportRow{Project: p}
			b.imports[p] = row
		}
		if first.Kind == store.Log {
			row.Logs++
		} else {
			row.Records++
		}
		if topic == NoTopic {
			row.Untopiced++
		}
		if !ownerRepo.MatchString(p) {
			b.scopes[p]++
		}
	}
	for _, tok := range n.Broken {
		b.add("broken", HygieneItem{Key: n.Key, Detail: tok})
	}
	switch status {
	case store.Expired:
		b.add("expired", HygieneItem{Key: n.Key, Detail: "mem verify " + n.Key})
	case store.Retired:
		b.add("retired", HygieneItem{Key: n.Key})
	case store.Superseded:
		d := "-> " + n.Successor
		if n.Successor == "" {
			d = ""
		}
		b.add("superseded", HygieneItem{Key: n.Key, Detail: d})
		if first.SupersededBy != "" {
			if _, ok := baseOf[first.SupersededBy]; !ok {
				b.add("dangling", HygieneItem{Key: n.Key, Detail: "superseded_by " + first.SupersededBy + " is not a record"})
			}
		} else if n.Successor == "" {
			b.add("dangling", HygieneItem{Key: n.Key, Detail: "superseded without a successor"})
		}
	case store.Current:
		if first.Kind == store.State {
			expires := now
			for _, r := range gr.recs {
				ttl := r.TTLDays
				if ttl <= 0 {
					ttl = store.DefaultStateTTL
				}
				if e := r.AsOf().Add(time.Duration(ttl) * 24 * time.Hour); e.After(expires) || expires.Equal(now) {
					expires = e
				}
			}
			if !now.Before(expires) {
				b.add("stale", HygieneItem{Key: n.Key, Detail: fmt.Sprintf("ttl %d d", n.TTLDays)})
			}
		}
		var why []string
		if topic == NoTopic {
			why = append(why, reasonNoTopic)
		}
		switch n.Vector {
		case "missing":
			why = append(why, reasonNoVector)
		case "stale":
			why = append(why, reasonStaleVec)
		}
		if len(why) > 0 {
			b.add("orphans", HygieneItem{Key: n.Key, Why: why})
		}
	}
	for _, r := range gr.recs {
		if r.VerifyCmd != "" && r.VerifiedAt.IsZero() && status == store.Current {
			b.add("verify", HygieneItem{Key: n.Key, Detail: "mem verify " + n.Key})
			break
		}
	}
	for _, r := range gr.recs {
		for _, t := range r.Triggers {
			if _, err := regexp.Compile(`^(?:` + t.Tool + `)$`); err != nil {
				b.add("triggers", HygieneItem{Key: n.Key, Detail: "tool " + t.Tool})
			} else if _, err := regexp.Compile(t.Regex); err != nil {
				b.add("triggers", HygieneItem{Key: n.Key, Detail: "regex " + t.Regex})
			}
		}
	}
	if n.Journal >= journalMin {
		b.add("journals", HygieneItem{Key: n.Key, Detail: fmt.Sprintf("%d", n.Journal)})
	}
}

func (b *baseHygiene) finish(g *Graph, _ time.Time) {
	for p, c := range b.scopes {
		b.add("scopes", HygieneItem{Key: "project:" + p, Detail: fmt.Sprintf("%d", c), Kind: scopeKind})
	}
	for _, items := range b.sections {
		sort.SliceStable(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	}
}

func (b *baseHygiene) noteDuplicate(a, c, note string) {
	b.add("duplicates", HygieneItem{Key: a, Detail: c + ": " + strings.TrimPrefix(note, "distinct:")})
}

func (g *Graph) hygiene(surfaced map[string]bool, covered int) Hygiene {
	h := Hygiene{Surfaced: Surfaced{WindowDays: surfacedDays, CoveredDays: covered}, Dropped: g.base.dropped}
	orphans := append([]HygieneItem{}, g.base.sections["orphans"]...)
	if covered >= surfacedDays {
		h.Surfaced.Applied = true
		have := map[string]int{}
		for i, it := range orphans {
			have[it.Key] = i
		}
		for _, n := range g.Records {
			m := g.meta[n.Key]
			if m.status != store.Current || m.core || surfaced[n.Key] {
				continue
			}
			if i, ok := have[n.Key]; ok {
				orphans[i].Why = append(append([]string{}, orphans[i].Why...), reasonUnsurface)
				continue
			}
			orphans = append(orphans, HygieneItem{Key: n.Key, Why: []string{reasonUnsurface}})
		}
		sort.SliceStable(orphans, func(i, j int) bool { return orphans[i].Key < orphans[j].Key })
	}
	distinct := map[string]bool{}
	for _, s := range sectionOrder {
		items := g.base.sections[s.id]
		if s.id == "orphans" {
			items = orphans
		}
		if items == nil {
			items = []HygieneItem{}
		}
		h.Sections = append(h.Sections, HygieneSection{ID: s.id, Counted: s.counted, Items: items})
		if s.counted {
			for _, it := range items {
				distinct[it.Key] = true
			}
		}
	}
	h.Count = len(distinct)
	h.Imports = make([]ImportRow, 0, len(g.base.imports))
	for _, r := range g.base.imports {
		h.Imports = append(h.Imports, *r)
	}
	sort.Slice(h.Imports, func(i, j int) bool {
		a, b := h.Imports[i], h.Imports[j]
		if a.Untopiced != b.Untopiced {
			return a.Untopiced > b.Untopiced
		}
		return a.Project < b.Project
	})
	return h
}

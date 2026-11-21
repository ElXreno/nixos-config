package ui

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"mem/internal/store"
)

const (
	maxInjected   = 10
	maxRefs       = 60
	diffContext   = 2
	diffCap       = 4_000_000
	maxDiffRunes  = 60_000
	tierStrong    = "strong"
	tierPointer   = "pointer"
	tierLexical   = "lexical"
	tierTool      = "tool"
	tierSession   = "session"
	tierCandidate = "candidate"
)

var bodyTokenRe = regexp.MustCompile("\\[\\[([a-z0-9_.#-]+)\\]\\]|`([a-z0-9][a-z0-9_.#-]{3,})`")

type linkOut struct {
	store.Link
	Dir      string   `json:"dir"`
	Other    string   `json:"other"`
	InGraph  bool     `json:"in_graph"`
	Resolved []string `json:"resolved,omitempty"`
	How      string   `json:"how,omitempty"`
	NonKey   bool     `json:"non_key,omitempty"`
}

type refOut struct {
	Keys []string `json:"keys"`
	How  string   `json:"how"`
}

type diffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
	N    int    `json:"n,omitempty"`
}

type versionOut struct {
	Version   int        `json:"version"`
	Reason    string     `json:"reason"`
	ChangedAt time.Time  `json:"changed_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Title     string     `json:"title"`
	Scopes    []string   `json:"scopes"`
	Status    string     `json:"status"`
	Body      string     `json:"body"`
	Meta      []string   `json:"meta,omitempty"`
	Diff      []diffLine `json:"diff"`
}

type vectorOut struct {
	Model     string `json:"model,omitempty"`
	State     string `json:"state"`
	Fresh     int    `json:"fresh"`
	Stale     int    `json:"stale"`
	Missing   int    `json:"missing"`
	Dim       int    `json:"dim,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type triggerOut struct {
	store.Trigger
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type injectedEntry struct {
	Time    string   `json:"time"`
	Session string   `json:"session"`
	Event   string   `json:"event"`
	Tier    string   `json:"tier"`
	Tool    string   `json:"tool,omitempty"`
	Project string   `json:"project,omitempty"`
	Input   string   `json:"input,omitempty"`
	Regex   string   `json:"regex,omitempty"`
	Cos     *float64 `json:"cos,omitempty"`
	Rerank  *float64 `json:"rerank,omitempty"`
}

type injectedOut struct {
	Total      int             `json:"total"`
	Candidates int             `json:"candidates"`
	Last       []injectedEntry `json:"last"`
}

type detailOut struct {
	Record   recordOut         `json:"record"`
	Topic    TopicOut          `json:"topic"`
	Links    []linkOut         `json:"links"`
	Refs     map[string]refOut `json:"refs"`
	History  []versionOut      `json:"history"`
	Vector   vectorOut         `json:"vector"`
	Triggers []triggerOut      `json:"triggers"`
	Injected injectedOut       `json:"injected"`
	Expires  string            `json:"expires,omitempty"`
}

func (s *Server) groupRecord(ctx context.Context, key string) (store.Record, []string, error) {
	rec, err := s.st.Get(ctx, key)
	switch {
	case err == nil:
		return rec, nil, nil
	case !errors.Is(err, store.ErrNotFound):
		return rec, nil, err
	}
	chunks, err := s.chunkKeys(ctx, key)
	if err != nil {
		return rec, nil, err
	}
	if len(chunks) == 0 {
		return rec, nil, store.ErrNotFound
	}
	var bodies []string
	for i, ck := range chunks {
		c, err := s.st.Get(ctx, ck)
		if err != nil {
			return rec, nil, err
		}
		if i == 0 {
			rec = c
		}
		bodies = append(bodies, c.Body)
	}
	rec.Key, rec.Body = key, strings.Join(bodies, "\n\n")
	return rec, chunks, nil
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	key, err := keyParam(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	ctx := r.Context()
	g, err := s.Graph(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	rec, chunks, err := s.groupRecord(ctx, key)
	if err != nil {
		s.fail(w, err)
		return
	}
	body := rec.Body
	out := detailOut{Record: recordOut{Record: rec, Body: &body, Chunks: chunks}, Links: []linkOut{}, Refs: map[string]refOut{}, History: []versionOut{}, Triggers: []triggerOut{}}
	if i, ok := g.byKey[key]; ok {
		out.Topic = g.Topics[g.Records[i].Topic]
	}
	if out.Links, err = s.detailLinks(ctx, g, key, chunks); err != nil {
		s.fail(w, err)
		return
	}
	out.Refs = bodyRefs(g, body)
	if out.History, err = s.detailHistory(ctx, key, rec); err != nil {
		s.fail(w, err)
		return
	}
	keys := []string{key}
	if len(chunks) > 0 {
		keys = chunks
	}
	if out.Vector, err = s.detailVector(ctx, keys); err != nil {
		s.fail(w, err)
		return
	}
	for _, t := range rec.Triggers {
		to := triggerOut{Trigger: t, OK: true}
		if _, err := regexp.Compile(`^(?:` + t.Tool + `)$`); err != nil {
			to.OK, to.Error = false, "tool: "+err.Error()
		} else if _, err := regexp.Compile(t.Regex); err != nil {
			to.OK, to.Error = false, "regex: "+err.Error()
		}
		out.Triggers = append(out.Triggers, to)
	}
	out.Injected = s.injections(key)
	if rec.Kind == store.State && rec.Status == store.Current {
		ttl := rec.TTLDays
		if ttl <= 0 {
			ttl = store.DefaultStateTTL
		}
		out.Expires = rec.AsOf().Add(time.Duration(ttl) * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) detailLinks(ctx context.Context, g *Graph, key string, chunks []string) ([]linkOut, error) {
	ls, err := s.linksOf(ctx, key)
	if err != nil {
		return nil, err
	}
	own := map[string]bool{key: true}
	for _, c := range chunks {
		own[c] = true
	}
	out := make([]linkOut, 0, len(ls))
	for _, l := range ls {
		lo := linkOut{Link: l, Dir: "out", Other: l.Dst}
		if !own[l.Src] {
			lo.Dir, lo.Other = "in", l.Src
		}
		base := lo.Other
		if _, ok := g.byKey[base]; !ok {
			base, _ = BaseKey(lo.Other)
		}
		if _, ok := g.byKey[base]; ok {
			lo.Other, lo.InGraph = base, true
		} else if l.Rel == store.Unresolved {
			if !g.res.valid(l.Dst) {
				lo.NonKey = true
			} else {
				lo.Resolved, lo.How = g.res.resolve(l.Dst)
			}
		}
		out = append(out, lo)
	}
	return out, nil
}

func bodyRefs(g *Graph, body string) map[string]refOut {
	out := map[string]refOut{}
	for _, m := range bodyTokenRe.FindAllStringSubmatch(body, -1) {
		tok := m[1] + m[2]
		if _, ok := out[tok]; ok || len(out) >= maxRefs {
			continue
		}
		if base, _ := BaseKey(tok); g.res.keys[base] {
			continue
		}
		if keys, how := g.res.resolve(tok); len(keys) > 0 {
			out[tok] = refOut{Keys: keys, How: how}
		}
	}
	return out
}

func (s *Server) detailHistory(ctx context.Context, key string, cur store.Record) ([]versionOut, error) {
	vs, err := s.st.History(ctx, key)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].Version < vs[j].Version })
	out := make([]versionOut, 0, len(vs))
	next := cur
	for i := len(vs) - 1; i >= 0; i-- {
		v := vs[i]
		vo := versionOut{Version: v.Version, Reason: v.Reason, ChangedAt: v.ChangedAt, UpdatedAt: v.UpdatedAt, Title: v.Title,
			Scopes: v.Scopes, Status: string(v.Status), Body: v.Body, Diff: lineDiff(v.Body, next.Body)}
		if v.Title != next.Title {
			vo.Meta = append(vo.Meta, "title: "+v.Title+" -> "+next.Title)
		}
		if strings.Join(v.Scopes, ",") != strings.Join(next.Scopes, ",") {
			vo.Meta = append(vo.Meta, "scopes: "+strings.Join(v.Scopes, ",")+" -> "+strings.Join(next.Scopes, ","))
		}
		if v.Status != next.Status {
			vo.Meta = append(vo.Meta, "status: "+string(v.Status)+" -> "+string(next.Status))
		}
		out = append(out, vo)
		next = v.Record
	}
	return out, nil
}

func (s *Server) detailVector(ctx context.Context, keys []string) (vectorOut, error) {
	out := vectorOut{State: "unknown"}
	model := s.modelName(ctx)
	if model == "" {
		return out, nil
	}
	out.Model = model
	prefix := s.opt.docPrefix()
	for _, k := range keys {
		rec, err := s.st.Get(ctx, k)
		if err != nil {
			return out, err
		}
		var dim int
		var sha, upd string
		err = s.st.DB().QueryRowContext(ctx, "SELECT dim, text_sha256, updated_at FROM vectors WHERE key = ? AND model = ?", k, model).Scan(&dim, &sha, &upd)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			out.Missing++
		case err != nil:
			return out, err
		case sha == store.TextSHA(store.DocText(prefix, rec)):
			out.Fresh++
			out.Dim = dim
		default:
			out.Stale++
			out.Dim = dim
		}
		if upd > out.UpdatedAt {
			out.UpdatedAt = upd
		}
	}
	switch {
	case out.Missing > 0:
		out.State = "missing"
	case out.Stale > 0:
		out.State = "stale"
	default:
		out.State = "fresh"
	}
	return out, nil
}

func (s *Server) injections(key string) injectedOut {
	out := injectedOut{Last: []injectedEntry{}}
	all, err := s.snapshot()
	if err != nil {
		return out
	}
	for i := len(all) - 1; i >= 0; i-- {
		t := all[i]
		var hit []injectedEntry
		add := func(k, tier string, cos, rr *float64) {
			if b, _ := BaseKey(k); b == key {
				hit = append(hit, injectedEntry{Time: t.Time, Session: t.Session, Event: t.Event, Tier: tier, Tool: t.Tool, Project: t.Project, Input: t.Input, Cos: cos, Rerank: rr})
			}
		}
		for _, k := range t.Strong {
			c := k.Cos
			add(k.Key, tierStrong, &c, k.Rerank)
		}
		for _, k := range t.Pointers {
			c := k.Cos
			add(k.Key, tierPointer, &c, k.Rerank)
		}
		for _, k := range t.Lexical {
			add(k, tierLexical, nil, nil)
		}
		for _, k := range t.Keys {
			tier := tierTool
			if t.Event == "session" {
				tier = tierSession
			}
			add(k, tier, nil, nil)
		}
		if len(hit) > 0 {
			for j := range hit {
				for _, f := range t.Fired {
					if b, _ := BaseKey(f.Key); b == key {
						hit[j].Regex = f.Regex
					}
				}
			}
			out.Total++
			if len(out.Last) < maxInjected {
				out.Last = append(out.Last, hit[0])
			}
			continue
		}
		for _, k := range t.Reranked {
			if b, _ := BaseKey(k.Key); b == key {
				out.Candidates++
				if len(out.Last) < maxInjected {
					c := k.Cos
					out.Last = append(out.Last, injectedEntry{Time: t.Time, Session: t.Session, Event: t.Event, Tier: tierCandidate, Project: t.Project, Cos: &c, Rerank: k.Rerank})
				}
				break
			}
		}
	}
	return out
}

func lineDiff(old, cur string) []diffLine {
	if len([]rune(old))+len([]rune(cur)) > maxDiffRunes {
		return []diffLine{{Op: "~", Text: "too large to diff"}}
	}
	a, b := splitLines(old), splitLines(cur)
	if len(a)*len(b) > diffCap {
		return []diffLine{{Op: "~", Text: "too large to diff"}}
	}
	n, m := len(a), len(b)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var raw []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			raw = append(raw, diffLine{Op: " ", Text: a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			raw = append(raw, diffLine{Op: "-", Text: a[i]})
			i++
		default:
			raw = append(raw, diffLine{Op: "+", Text: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		raw = append(raw, diffLine{Op: "-", Text: a[i]})
	}
	for ; j < m; j++ {
		raw = append(raw, diffLine{Op: "+", Text: b[j]})
	}
	return collapseDiff(raw)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func collapseDiff(raw []diffLine) []diffLine {
	keep := make([]bool, len(raw))
	for i, l := range raw {
		if l.Op == " " {
			continue
		}
		for k := max(0, i-diffContext); k <= min(len(raw)-1, i+diffContext); k++ {
			keep[k] = true
		}
	}
	out := []diffLine{}
	skipped := 0
	flush := func() {
		if skipped > 0 {
			out = append(out, diffLine{Op: "skip", N: skipped})
			skipped = 0
		}
	}
	for i, l := range raw {
		if keep[i] {
			flush()
			out = append(out, l)
		} else {
			skipped++
		}
	}
	flush()
	return out
}

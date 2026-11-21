package ui

import (
	"net/http"
)

type statsOut struct {
	Built     string         `json:"built"`
	Host      string         `json:"host"`
	DB        string         `json:"db"`
	Schema    int            `json:"schema"`
	Rows      int            `json:"rows"`
	Nodes     int            `json:"nodes"`
	ByKind    map[string]int `json:"by_kind"`
	ByStatus  map[string]int `json:"by_status"`
	Links     map[string]int `json:"links"`
	Pairs     int            `json:"link_pairs"`
	History   int            `json:"history"`
	Topics    int            `json:"topics"`
	Untopiced int            `json:"untopiced"`
	Core      int            `json:"core"`
	Hygiene   map[string]int `json:"hygiene"`
	Newest    string         `json:"newest"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := s.Graph(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := statsOut{Built: g.Built, Host: g.Host, DB: g.DirShort, Nodes: len(g.Records), Pairs: g.Links, Rows: g.Rows}
	for _, t := range g.Topics {
		if t.ID == NoTopic {
			out.Untopiced = t.Size
		} else {
			out.Topics++
		}
	}
	if out.Schema, err = s.st.SchemaVersion(ctx); err != nil {
		s.fail(w, err)
		return
	}
	if out.ByKind, err = s.countBy(ctx, "SELECT kind, count(*) FROM records GROUP BY kind"); err != nil {
		s.fail(w, err)
		return
	}
	if out.ByStatus, err = s.countBy(ctx, "SELECT status, count(*) FROM records GROUP BY status"); err != nil {
		s.fail(w, err)
		return
	}
	if out.Links, err = s.countBy(ctx, "SELECT rel, count(*) FROM links GROUP BY rel"); err != nil {
		s.fail(w, err)
		return
	}
	err = s.st.DB().QueryRowContext(ctx, `SELECT (SELECT count(*) FROM history), (SELECT count(*) FROM records WHERE core = 1 AND status = 'current'),
		(SELECT coalesce(max(updated_at), '') FROM records)`).Scan(&out.History, &out.Core, &out.Newest)
	if err != nil {
		s.fail(w, err)
		return
	}
	h := s.hygieneOf(g)
	out.Hygiene = map[string]int{"count": h.Count}
	for _, sec := range h.Sections {
		out.Hygiene[sec.ID] = len(sec.Items)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleHygiene(w http.ResponseWriter, r *http.Request) {
	g, err := s.Graph(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.hygieneOf(g))
}

func (s *Server) hygieneOf(g *Graph) Hygiene {
	surfaced, covered := s.surfaced(surfacedDays)
	return g.hygiene(surfaced, covered)
}

type topicOut struct {
	ID       string         `json:"id"`
	Index    int            `json:"index"`
	Label    string         `json:"label"`
	Keywords []string       `json:"keywords,omitempty"`
	Size     int            `json:"size"`
	Kinds    map[string]int `json:"kinds"`
	Members  []string       `json:"members,omitempty"`
}

func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	g, err := s.Graph(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	withMembers := r.URL.Query().Get("members") == "1"
	topics := make([]topicOut, len(g.Topics))
	for i, t := range g.Topics {
		topics[i] = topicOut{ID: t.ID, Index: i, Label: t.Label, Keywords: t.Keywords, Kinds: map[string]int{}}
	}
	for _, n := range g.Records {
		t := &topics[n.Topic]
		t.Size++
		t.Kinds[n.Type]++
		if withMembers {
			t.Members = append(t.Members, n.Key)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

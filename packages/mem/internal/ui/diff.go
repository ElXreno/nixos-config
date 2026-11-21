package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"sync"
)

const revRingSize = 16

type revEntry struct {
	rev    string
	nodes  map[string]uint64
	topics []string
	thash  map[string]uint64
}

type revSnap struct {
	g     *Graph
	rev   string
	out   graphOut
	entry *revEntry
}

type revStore struct {
	mu   sync.Mutex
	cur  *revSnap
	ring []*revEntry
}

type graphDiff struct {
	From   string            `json:"from"`
	Rev    string            `json:"rev"`
	Built  string            `json:"built"`
	Links  int               `json:"links"`
	Rows   int               `json:"rows"`
	Layout layoutOut         `json:"layout"`
	Topics []json.RawMessage `json:"topics"`
	Add    []graphNode       `json:"add"`
	Set    []graphNode       `json:"set"`
	Del    []string          `json:"del"`
}

func hash64(parts ...[]byte) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write(p)
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

func entryOf(out graphOut) *revEntry {
	e := &revEntry{nodes: make(map[string]uint64, len(out.Records)), topics: make([]string, len(out.Topics)), thash: make(map[string]uint64, len(out.Topics))}
	whole := fnv.New64a()
	put := func(parts ...string) {
		for _, p := range parts {
			_, _ = whole.Write([]byte(p))
			_, _ = whole.Write([]byte{0})
		}
	}
	lay, _ := json.Marshal(out.Layout)
	put(out.Built, fmt.Sprint(out.Links), fmt.Sprint(out.Rows), string(lay))
	for i, t := range out.Topics {
		raw, _ := json.Marshal(t)
		e.topics[i] = t.ID
		e.thash[t.ID] = hash64([]byte(t.ID), raw)
		put(t.ID, fmt.Sprint(e.thash[t.ID]))
	}
	for _, n := range out.Records {
		topic := ""
		if n.Topic >= 0 && n.Topic < len(out.Topics) {
			topic = out.Topics[n.Topic].ID
		}
		n.Topic = 0
		raw, _ := json.Marshal(n)
		h := hash64(raw, []byte(topic))
		e.nodes[n.Key] = h
		put(n.Key, fmt.Sprint(h))
	}
	e.rev = fmt.Sprintf("%016x", whole.Sum64())
	return e
}

func (s *Server) revision(ctx context.Context) (*revSnap, error) {
	g, err := s.Graph(ctx)
	if err != nil {
		return nil, err
	}
	rv := &s.rv
	rv.mu.Lock()
	defer rv.mu.Unlock()
	if rv.cur != nil && rv.cur.g == g {
		return rv.cur, nil
	}
	out := s.graphOut(ctx, g)
	if out.Layout.State != layoutStateInfo {
		return &revSnap{g: g, out: out}, nil
	}
	e := entryOf(out)
	out.Rev = e.rev
	snap := &revSnap{g: g, rev: e.rev, out: out, entry: e}
	rv.cur = snap
	for i, old := range rv.ring {
		if old.rev == e.rev {
			rv.ring = append(rv.ring[:i], rv.ring[i+1:]...)
			break
		}
	}
	rv.ring = append(rv.ring, e)
	if len(rv.ring) > revRingSize {
		rv.ring = rv.ring[len(rv.ring)-revRingSize:]
	}
	return snap, nil
}

func (rv *revStore) lookup(rev string) *revEntry {
	rv.mu.Lock()
	defer rv.mu.Unlock()
	for _, e := range rv.ring {
		if e.rev == rev {
			return e
		}
	}
	return nil
}

func diffOf(from *revEntry, cur *revSnap) graphDiff {
	d := graphDiff{From: from.rev, Rev: cur.rev, Built: cur.out.Built, Links: cur.out.Links, Rows: cur.out.Rows, Layout: cur.out.Layout,
		Topics: make([]json.RawMessage, len(cur.out.Topics)), Add: []graphNode{}, Set: []graphNode{}, Del: []string{}}
	for i, t := range cur.out.Topics {
		if old, ok := from.thash[t.ID]; ok && old == cur.entry.thash[t.ID] {
			d.Topics[i], _ = json.Marshal(map[string]any{"id": t.ID, "same": true})
			continue
		}
		d.Topics[i], _ = json.Marshal(t)
	}
	for _, n := range cur.out.Records {
		old, ok := from.nodes[n.Key]
		switch {
		case !ok:
			d.Add = append(d.Add, n)
		case old != cur.entry.nodes[n.Key]:
			d.Set = append(d.Set, n)
		}
	}
	for k := range from.nodes {
		if _, ok := cur.entry.nodes[k]; !ok {
			d.Del = append(d.Del, k)
		}
	}
	sort.Strings(d.Del)
	return d
}

func (s *Server) handleGraphDiff(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	if from == "" {
		writeError(w, http.StatusBadRequest, "from must be a revision returned by /api/graph")
		return
	}
	snap, err := s.revision(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if snap.entry == nil {
		writeError(w, http.StatusServiceUnavailable, "layout is building")
		return
	}
	old := s.rv.lookup(from)
	if old == nil {
		writeError(w, http.StatusGone, "unknown revision")
		return
	}
	writeJSON(w, http.StatusOK, diffOf(old, snap))
}

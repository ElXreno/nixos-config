package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mem/internal/recall"
)

const (
	layoutSaveDelay = 2 * time.Second
	layoutWaitCold  = 250 * time.Millisecond
	layoutStateInfo = "ready"
	layoutStateWait = "building"

	sparseMinRecords = 16
)

func layoutFileName(dbPath string) string {
	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}
	sum := sha256.Sum256([]byte(dbPath))
	return "graph-layout-" + hex.EncodeToString(sum[:4]) + ".json"
}

func readLayoutFile(path string) *layoutData {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	d := &layoutData{}
	if err := json.Unmarshal(raw, d); err != nil || !d.valid() {
		return nil
	}
	return d
}

func encodeLayout(d *layoutData) ([]byte, string, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, "", err
	}
	h := fnv.New64a()
	_, _ = h.Write(raw)
	return raw, fmt.Sprintf("%016x", h.Sum64()), nil
}

func writeLayoutFile(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".graph-layout-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

type layoutMgr struct {
	mu       sync.Mutex
	path     string
	relayout bool
	tried    bool
	data     *layoutData
	rev      string
	fp       string
	building chan struct{}
	dirty    bool
	timer    *time.Timer
	delay    time.Duration
}

func newLayoutMgr(stateDir, dbPath string, relayout bool) *layoutMgr {
	m := &layoutMgr{relayout: relayout, delay: layoutSaveDelay}
	if stateDir != "" {
		m.path = filepath.Join(stateDir, layoutFileName(dbPath))
	}
	return m
}

type layoutOut struct {
	Rev        string `json:"rev"`
	State      string `json:"state"`
	Stale      bool   `json:"stale,omitempty"`
	StaleCount int    `json:"staleTopics,omitempty"`
}

type layoutView struct {
	pos  []layoutNode
	has  []bool
	geom map[string]*topicGeom
	out  layoutOut
}

func (m *layoutMgr) load() {
	if m.tried {
		return
	}
	m.tried = true
	if m.path == "" || m.relayout {
		return
	}
	if d := readLayoutFile(m.path); d != nil {
		m.data = d
		_, m.rev, _ = encodeLayout(d)
	}
}

func (m *layoutMgr) install(d *layoutData, immediate bool) {
	raw, rev, err := encodeLayout(d)
	if err != nil {
		return
	}
	m.data, m.rev = d, rev
	if m.path == "" {
		return
	}
	if immediate {
		if m.timer != nil {
			m.timer.Stop()
			m.timer = nil
		}
		m.dirty = false
		_ = writeLayoutFile(m.path, raw)
		return
	}
	m.dirty = true
	if m.timer == nil {
		m.timer = time.AfterFunc(m.delay, m.Flush)
	} else {
		m.timer.Reset(m.delay)
	}
}

func (m *layoutMgr) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if !m.dirty || m.path == "" || m.data == nil {
		return
	}
	raw, _, err := encodeLayout(m.data)
	if err != nil {
		return
	}
	m.dirty = false
	_ = writeLayoutFile(m.path, raw)
}

func (m *layoutMgr) viewLocked(g *Graph, in *layoutInput) *layoutView {
	stale := m.data.staleTopics()
	v := &layoutView{
		pos:  make([]layoutNode, len(g.Records)),
		has:  make([]bool, len(g.Records)),
		geom: m.data.geometry(in),
		out:  layoutOut{Rev: m.rev, State: layoutStateInfo, Stale: stale > 0, StaleCount: stale},
	}
	for i, n := range g.Records {
		v.pos[i], v.has[i] = m.data.Nodes[n.Key]
	}
	return v
}

func sparseLayout(d *layoutData, g *Graph) bool {
	if len(g.Records) < sparseMinRecords {
		return false
	}
	covered := 0
	for _, n := range g.Records {
		if _, ok := d.Nodes[n.Key]; ok {
			covered++
		}
	}
	return covered*2 < len(g.Records)
}

func (s *Server) layoutVectors(g *Graph) map[string][]float32 {
	if g.vec.Model == "" {
		return nil
	}
	entries, err := s.vectorEntries(context.Background(), g.vec.Model, g.fp)
	if err != nil || len(entries) == 0 {
		return nil
	}
	return meanVectors(entries)
}

func meanVectors(entries []recall.Entry) map[string][]float32 {
	sum := map[string][]float32{}
	for _, e := range entries {
		base, _ := BaseKey(e.Key)
		if s, ok := sum[base]; ok {
			if len(s) != len(e.Vec) {
				continue
			}
			for i, v := range e.Vec {
				s[i] += v
			}
			continue
		}
		sum[base] = append([]float32(nil), e.Vec...)
	}
	for k, v := range sum {
		var n float64
		for _, x := range v {
			n += float64(x) * float64(x)
		}
		if n == 0 {
			delete(sum, k)
			continue
		}
		inv := float32(1 / math.Sqrt(n))
		for i := range v {
			v[i] *= inv
		}
	}
	return sum
}

func (s *Server) layoutFor(ctx context.Context, g *Graph) (*layoutView, *layoutOut) {
	m := s.lay
	m.mu.Lock()
	m.load()
	if m.data != nil && m.fp != g.fp && sparseLayout(m.data, g) {
		m.data, m.rev = nil, ""
	}
	if m.data == nil {
		if m.building == nil {
			done := make(chan struct{})
			m.building = done
			go func() {
				defer close(done)
				in := newLayoutInput(g, s.layoutVectors(g))
				d := computeFullLayout(in)
				m.mu.Lock()
				m.install(d, true)
				m.fp = g.fp
				m.building = nil
				m.mu.Unlock()
			}()
		}
		done := m.building
		m.mu.Unlock()
		select {
		case <-done:
		case <-time.After(layoutWaitCold):
		case <-ctx.Done():
		}
		m.mu.Lock()
		if m.data == nil {
			m.mu.Unlock()
			return nil, &layoutOut{State: layoutStateWait}
		}
	}
	defer m.mu.Unlock()
	var in *layoutInput
	if m.fp != g.fp {
		in = newLayoutInput(g, nil)
		if needsVectors(m.data, in) {
			in = newLayoutInput(g, s.layoutVectors(g))
		}
		if res := syncLayout(m.data, in); res.changed {
			m.install(m.data, false)
		}
		m.fp = g.fp
	}
	if in == nil {
		in = newLayoutInput(g, nil)
	}
	return m.viewLocked(g, in), nil
}

func needsVectors(d *layoutData, in *layoutInput) bool {
	for _, n := range in.nodes {
		if _, ok := d.Nodes[n.key]; !ok {
			return true
		}
	}
	return false
}

func (s *Server) FlushLayout() { s.lay.Flush() }

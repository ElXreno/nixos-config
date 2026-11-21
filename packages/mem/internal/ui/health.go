package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/counter"
	"mem/internal/hook"
	"mem/internal/recall"
	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	probeTTL     = 10 * time.Second
	probeTimeout = 1500 * time.Millisecond
	errorWindow  = 24 * time.Hour
	maxErrLines  = 5
)

type probeResult struct {
	At     time.Time
	OK     bool
	Ms     int64
	Status int
	Err    string
}

type svcOut struct {
	Configured bool   `json:"configured"`
	URL        string `json:"url,omitempty"`
	Reachable  bool   `json:"reachable"`
	OK         bool   `json:"ok"`
	Ms         int64  `json:"ms"`
	Status     int    `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at,omitempty"`
}

type latency struct {
	N   int   `json:"n"`
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	P99 int64 `json:"p99"`
	Max int64 `json:"max"`
}

type errLine struct {
	Time    string `json:"time"`
	Event   string `json:"event"`
	Message string `json:"message"`
}

type hookHealth struct {
	Turns         int                `json:"turns"`
	Since         string             `json:"since,omitempty"`
	ByEvent       map[string]int     `json:"by_event"`
	ByStatus      map[string]int     `json:"by_status"`
	BySource      map[string]int     `json:"by_source"`
	InjectedShare float64            `json:"injected_share"`
	AvgChars      float64            `json:"avg_chars"`
	Latency       map[string]latency `json:"latency"`
	Errors24h     int                `json:"errors_24h"`
	LastErrors    []errLine          `json:"last_errors"`
}

type vectorHealth struct {
	Model   string         `json:"model,omitempty"`
	Dim     int            `json:"dim,omitempty"`
	Rows    int            `json:"rows"`
	Current int            `json:"current"`
	Fresh   int            `json:"fresh"`
	Stale   int            `json:"stale"`
	Missing int            `json:"missing"`
	ByKind  map[string]int `json:"missing_by_kind"`
}

type histogram struct {
	Edges  []float64 `json:"edges"`
	Counts []int     `json:"counts"`
}

type sweepRow struct {
	T       float64 `json:"t"`
	Strong  int     `json:"strong"`
	Pointer int     `json:"pointer"`
	Shadow  int     `json:"shadow"`
}

type calibration struct {
	StrongCos  histogram  `json:"strong_cos"`
	PointerCos histogram  `json:"pointer_cos"`
	ShadowCos  histogram  `json:"shadow_cos"`
	Rerank     histogram  `json:"rerank"`
	Sweep      []sweepRow `json:"sweep"`
	ShadowOnly int        `json:"shadow_only"`
	Shadow     int        `json:"shadow_lines"`
}

type healthOut struct {
	Embedder    svcOut             `json:"embedder"`
	Reranker    svcOut             `json:"reranker"`
	Counters    map[string]int64   `json:"counters"`
	Vectors     vectorHealth       `json:"vectors"`
	Hook        hookHealth         `json:"hook"`
	Gates       map[string]float64 `json:"gates"`
	Calibration calibration        `json:"calibration"`
	Store       map[string]any     `json:"store"`
}

var (
	cosEdges        = []float64{0, 0.3, 0.4, 0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 0.8, 0.85, 0.9, 1.0001}
	sweepThresholds = []float64{0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 0.8, 0.85, 0.9}
	rerankEdges     = []float64{0, 0.01, 0.05, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99, 1.0001}
)

func newHistogram(edges []float64) histogram {
	return histogram{Edges: edges, Counts: make([]int, len(edges)-1)}
}

func (h *histogram) add(v float64) {
	for i := len(h.Edges) - 2; i >= 0; i-- {
		if v >= h.Edges[i] {
			h.Counts[i]++
			return
		}
	}
}

func (s *Server) probe(name, base string) svcOut {
	out := svcOut{Configured: base != ""}
	if base == "" {
		return out
	}
	if u, err := url.Parse(base); err == nil {
		out.URL = u.Scheme + "://" + u.Host
	}
	s.probeMu.Lock()
	cached, ok := s.probes[name]
	s.probeMu.Unlock()
	if !ok || s.opt.now().Sub(cached.At) > probeTTL {
		cached = s.doProbe(base)
		s.probeMu.Lock()
		s.probes[name] = cached
		s.probeMu.Unlock()
	}
	out.Reachable, out.OK, out.Ms, out.Status, out.Error = cached.Err == "", cached.OK, cached.Ms, cached.Status, cached.Err
	out.CheckedAt = cached.At.Format(time.RFC3339)
	return out
}

func (s *Server) doProbe(base string) probeResult {
	res := probeResult{At: s.opt.now()}
	client := s.opt.HTTP
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/health", nil)
	if err != nil {
		res.Err = "bad url"
		return res
	}
	t0 := time.Now()
	resp, err := client.Do(req)
	res.Ms = time.Since(t0).Milliseconds()
	if err != nil {
		res.Err = "unreachable"
		if ctx.Err() != nil {
			res.Err = "timeout"
		}
		return res
	}
	_ = resp.Body.Close()
	res.Status, res.OK = resp.StatusCode, resp.StatusCode < 400
	return res
}

func percentiles(v []int64) latency {
	if len(v) == 0 {
		return latency{}
	}
	s := append([]int64{}, v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	at := func(p float64) int64 { return s[min(len(s)-1, int(p*float64(len(s)-1)+0.5))] }
	return latency{N: len(s), P50: at(0.5), P90: at(0.9), P99: at(0.99), Max: s[len(s)-1]}
}

func (s *Server) hookHealth(now time.Time) hookHealth {
	h := hookHealth{ByEvent: map[string]int{}, ByStatus: map[string]int{}, BySource: map[string]int{}, Latency: map[string]latency{}, LastErrors: []errLine{}}
	all, _ := s.snapshot()
	var total, embed, rerank []int64
	var first time.Time
	injected, chars, counted := 0, 0, 0
	for _, t := range all {
		h.Turns++
		h.ByEvent[t.Event]++
		h.ByStatus[t.Status]++
		h.BySource[t.Origin]++
		if first.IsZero() || t.at.Before(first) {
			first = t.at
		}
		if t.Status == "error" && now.Sub(t.at) <= errorWindow {
			h.Errors24h++
		}
		if t.Event != statefile.EventPrompt || t.Status == "skipped" || t.Status == "error" {
			continue
		}
		counted++
		if t.Status == "inject" {
			injected++
			chars += t.Chars
		}
		if t.Origin == sourceTurns {
			total = append(total, t.TotalMs)
			if t.Embed != "" {
				embed = append(embed, t.EmbedMs)
			}
			if t.Rerank != "" {
				rerank = append(rerank, t.RerankMs)
			}
		}
	}
	if !first.IsZero() {
		h.Since = first.Format(time.RFC3339)
	}
	if counted > 0 {
		h.InjectedShare = float64(injected) / float64(counted)
	}
	if injected > 0 {
		h.AvgChars = float64(chars) / float64(injected)
	}
	h.Latency["total_ms"], h.Latency["embed_ms"], h.Latency["rerank_ms"] = percentiles(total), percentiles(embed), percentiles(rerank)
	h.LastErrors = s.logErrors()
	return h
}

func (s *Server) logErrors() []errLine {
	out := []errLine{}
	if s.opt.StateDir == "" {
		return out
	}
	lines, _, err := statefile.Read(s.opt.StateDir, statefile.HookLogName, nil)
	if err != nil {
		return out
	}
	for i := len(lines) - 1; i >= 0 && len(out) < maxErrLines; i-- {
		f := strings.SplitN(string(lines[i].Data), " ", 6)
		if len(f) < 5 || f[3] != logError {
			continue
		}
		msg := ""
		if len(f) == 6 {
			msg = f[5]
		}
		out = append(out, errLine{Time: f[0], Event: f[1], Message: clipRunes(msg, 160)})
	}
	return out
}

func (s *Server) calibrate() calibration {
	c := calibration{StrongCos: newHistogram(cosEdges), PointerCos: newHistogram(cosEdges), ShadowCos: newHistogram(cosEdges), Rerank: newHistogram(rerankEdges)}
	all, _ := s.snapshot()
	var strong, pointer, shadow []float64
	for _, t := range all {
		for _, k := range t.Strong {
			if k.Cos == 0 {
				continue
			}
			c.StrongCos.add(k.Cos)
			strong = append(strong, k.Cos)
		}
		for _, k := range t.Pointers {
			if k.Cos == 0 {
				continue
			}
			c.PointerCos.add(k.Cos)
			pointer = append(pointer, k.Cos)
		}
		for _, k := range t.Reranked {
			if k.Rerank != nil {
				c.Rerank.add(*k.Rerank)
			}
		}
		if t.Shadow != nil {
			c.Shadow++
			for _, k := range t.Shadow.cands {
				if k.Cos == 0 {
					continue
				}
				c.ShadowCos.add(k.Cos)
				shadow = append(shadow, k.Cos)
			}
			c.ShadowOnly += len(t.Shadow.Only)
		}
	}
	for _, th := range sweepThresholds {
		c.Sweep = append(c.Sweep, sweepRow{T: th, Strong: countAtLeast(strong, th), Pointer: countAtLeast(pointer, th), Shadow: countAtLeast(shadow, th)})
	}
	return c
}

func countAtLeast(vs []float64, th float64) int {
	n := 0
	for _, v := range vs {
		if v >= th {
			n++
		}
	}
	return n
}

func (s *Server) vectorHealth(g *Graph) vectorHealth {
	return vectorHealth(g.vec)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := s.Graph(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := healthOut{
		Embedder: s.probe("embed", s.opt.EmbedURL),
		Reranker: s.probe("rerank", s.opt.RerankURL),
		Counters: map[string]int64{},
		Vectors:  s.vectorHealth(g),
		Hook:     s.hookHealth(s.opt.now()),
		Gates: map[string]float64{"cos": recall.DefaultVectorSettings().Reg.MinCos, "rerank_text": recall.DefaultRerankText,
			"rerank_pointer": recall.DefaultRerankPointer},
		Calibration: s.calibrate(),
		Store:       map[string]any{"rows": g.Rows, "nodes": len(g.Records), "links": g.Links, "built": g.Built},
	}
	if s.opt.StateDir != "" {
		if c, err := counter.Counts(filepath.Join(s.opt.StateDir, hook.CountersDir)); err == nil {
			out.Counters = c
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFingerprint(w http.ResponseWriter, r *http.Request) {
	fp, err := s.fingerprint(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	sum := sha256.Sum256([]byte(fp + "|" + s.opt.now().Format(time.DateOnly)))
	writeJSON(w, http.StatusOK, map[string]string{"fingerprint": hex.EncodeToString(sum[:8])})
}

type coreRow struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind"`
	Scopes     []string `json:"scopes"`
	Chars      int      `json:"chars"`
	InFile     bool     `json:"in_file"`
	VerifiedAt string   `json:"verified_at,omitempty"`
	AsOf       string   `json:"as_of"`
}

func (s *Server) handleCore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var recs []store.Record
	var err error
	q := r.URL.Query()
	if q.Get("project") != "" || q.Get("host") != "" {
		h := store.Here{}
		if p := q.Get("project"); p != "" {
			h.Project = store.ProjectScope(strings.ToLower(p))
		}
		if hs := q.Get("host"); hs != "" {
			h.Host = store.HostScope(strings.ToLower(hs))
		}
		recs, err = s.st.CoreAt(ctx, h)
	} else {
		recs, err = s.st.Core(ctx, nil)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	rows := make([]coreRow, 0, len(recs))
	var file []store.Record
	for _, rec := range recs {
		row := coreRow{Key: rec.Key, Title: rec.Title, Kind: string(rec.Kind), Scopes: rec.Scopes, Chars: utf8.RuneCountInString(store.RenderRecord(rec)),
			InFile: rec.HasScope(store.GlobalScope), AsOf: rec.AsOf().UTC().Format(time.RFC3339)}
		if !rec.VerifiedAt.IsZero() {
			row.VerifiedAt = rec.VerifiedAt.UTC().Format(time.RFC3339)
		}
		if row.InFile {
			file = append(file, rec)
		}
		rows = append(rows, row)
	}
	size := 0
	if text, err := store.RenderCore(file, 1<<30); err == nil {
		size = utf8.RuneCountInString(text)
	}
	writeJSON(w, http.StatusOK, map[string]any{"budget": store.CoreBudget, "size": size, "over": size > store.CoreBudget, "count": len(rows), "records": rows})
}

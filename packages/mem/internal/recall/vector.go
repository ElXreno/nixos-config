package recall

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"mem/internal/legacy"
	"mem/internal/store"
)

const (
	NoMargin           = -3.0
	DefaultVecDepth    = 60
	DefaultVecWeight   = 1.0
	DefaultWordsWeight = 1.0
	DefaultQueryPrefix = "Query: "
	DefaultDocPrefix   = "Document: "
	DefaultPointerTake = 3
)

type Pool string

const (
	PoolNone     Pool = ""
	PoolReg      Pool = "reg"
	PoolFusedReg Pool = "fused-reg"
	PoolAll      Pool = "all"
	PoolFusedAll Pool = "fused-all"
)

var Pools = []Pool{PoolReg, PoolFusedReg, PoolAll, PoolFusedAll}

func ParsePool(s string) (Pool, error) {
	if s == "" || s == "lexical" {
		return PoolNone, nil
	}
	p := Pool(s)
	if !slices.Contains(Pools, p) {
		return PoolNone, fmt.Errorf("pool %q is not lexical, reg, fused-reg, all or fused-all", s)
	}
	return p, nil
}

func (p Pool) Fused() bool { return p == PoolFusedReg || p == PoolFusedAll }

func (p Pool) Logs() bool { return p == PoolAll || p == PoolFusedAll }

type KindGate struct {
	Take      int     `json:"take"`
	MinCos    float64 `json:"min_cos"`
	MinMargin float64 `json:"min_margin"`

	PointerCos float64 `json:"pointer_cos"`
}

func OffGate() KindGate { return KindGate{MinMargin: NoMargin} }

type VHit struct {
	Key     string  `json:"key"`
	File    string  `json:"file"`
	Cos     float64 `json:"cos"`
	Log     bool    `json:"log"`
	Expired bool    `json:"expired,omitempty"`
}

func (g KindGate) Apply(list []VHit) []VHit {
	if g.Take <= 0 || len(list) == 0 || list[0].Cos < g.MinCos {
		return nil
	}
	if g.MinMargin > NoMargin {
		second := 0.0
		if len(list) > 1 {
			second = list[1].Cos
		}
		if list[0].Cos-second < g.MinMargin {
			return nil
		}
	}
	var out []VHit
	for i := 0; i < len(list) && i < g.Take; i++ {
		if list[i].Cos >= g.MinCos {
			out = append(out, list[i])
		}
	}
	return out
}

func (g KindGate) Pointers(list []VHit, take int, skip func(VHit) bool) []VHit {
	if g.PointerCos <= 0 || take <= 0 {
		return nil
	}
	var out []VHit
	for _, h := range list {
		if h.Cos < g.PointerCos || h.Cos >= g.MinCos || skip != nil && skip(h) {
			continue
		}
		out = append(out, h)
		if len(out) == take {
			break
		}
	}
	return out
}

func FileID(key string, kind store.Kind, source string) string {
	if p, ok := strings.CutPrefix(source, legacy.SourcePrefix); ok {
		return strings.TrimSuffix(filepath.Base(p), ".md")
	}
	if kind == store.Log {
		if i := strings.LastIndexByte(key, '#'); i > 0 {
			return key[:i]
		}
	}
	return key
}

type Entry struct {
	Key     string
	File    string
	Log     bool
	Expired bool
	Vec     []float32
}

type Index struct {
	Dim     int
	Entries []Entry
	by      map[string]int
}

func NewIndex(entries []Entry) *Index {
	ix := &Index{Entries: entries, by: make(map[string]int, len(entries))}
	for i, e := range entries {
		ix.by[e.Key] = i
		if ix.Dim == 0 {
			ix.Dim = len(e.Vec)
		}
	}
	return ix
}

var (
	ErrDimension = errors.New("query vector and stored vectors differ in dimension")
	ErrQuery     = errors.New("unusable query vector")
	ErrNoVectors = errors.New("no stored vectors")
)

func LoadIndex(ctx context.Context, s *store.Store, model string, q store.Query) (*Index, error) {
	var entries []Entry
	dim := 0
	err := s.ScanVectors(ctx, model, q, func(v store.VecScan) error {
		if dim == 0 {
			dim = v.Dim
		}
		if v.Dim != dim {
			return fmt.Errorf("%w: model %s holds dimensions %d and %d", ErrDimension, model, dim, v.Dim)
		}
		entries = append(entries, Entry{Key: v.Key, File: FileID(v.Key, v.Kind, v.Source), Log: v.Kind == store.Log, Expired: v.Status == store.Expired, Vec: store.DecodeVec(v.Blob, nil)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrNoVectors
	}
	return NewIndex(entries), nil
}

func (ix *Index) Has(key string) bool {
	_, ok := ix.by[key]
	return ok
}

func (ix *Index) fileOf(key string) string {
	if i, ok := ix.by[key]; ok {
		return ix.Entries[i].File
	}
	return key
}

func CheckQuery(q []float32) error {
	if len(q) == 0 {
		return fmt.Errorf("%w: empty", ErrQuery)
	}
	var s float64
	for _, x := range q {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return fmt.Errorf("%w: non-finite component", ErrQuery)
		}
		s += float64(x) * float64(x)
	}
	if !(s > 0) {
		return fmt.Errorf("%w: zero vector", ErrQuery)
	}
	return nil
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func (ix *Index) Score(q []float32) ([]VHit, error) {
	if err := CheckQuery(q); err != nil {
		return nil, err
	}
	if len(q) != ix.Dim {
		return nil, fmt.Errorf("%w: %d against %d", ErrDimension, len(q), ix.Dim)
	}
	out := make([]VHit, len(ix.Entries))
	for i, e := range ix.Entries {
		out[i] = VHit{Key: e.Key, File: e.File, Cos: dot(q, e.Vec), Log: e.Log, Expired: e.Expired}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cos != out[j].Cos {
			return out[i].Cos > out[j].Cos
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func dedupeFiles(list []VHit, n int) []VHit {
	seen := map[string]bool{}
	var out []VHit
	for _, h := range list {
		if seen[h.File] {
			continue
		}
		seen[h.File] = true
		out = append(out, h)
		if len(out) == n {
			break
		}
	}
	return out
}

func splitKinds(list []VHit) (reg, logs []VHit) {
	for _, h := range list {
		if h.Log {
			logs = append(logs, h)
		} else {
			reg = append(reg, h)
		}
	}
	return reg, logs
}

type Weights struct {
	Words float64 `json:"words"`
	Tri   float64 `json:"tri"`
	Vec   float64 `json:"vec"`
}

func DefaultWeights() Weights {
	return Weights{Words: DefaultWordsWeight, Tri: store.TriWeight, Vec: DefaultVecWeight}
}

type Fused struct {
	Key   string
	Score float64
	BM25  float64
}

func Fuse(words, tri []store.ListHit, vecKeys []string, w Weights, limit int) []Fused {
	type acc struct {
		wr, tr, vr int
		bm         float64
	}
	by := map[string]*acc{}
	get := func(k string) *acc {
		a := by[k]
		if a == nil {
			a = &acc{}
			by[k] = a
		}
		return a
	}
	for _, h := range words {
		a := get(h.Key)
		a.wr = h.Rank
		a.bm += h.Score
	}
	for _, h := range tri {
		a := get(h.Key)
		a.tr = h.Rank
		a.bm += h.Score
	}
	if w.Vec > 0 {
		for i, k := range vecKeys {
			get(k).vr = i + 1
		}
	}
	out := make([]Fused, 0, len(by))
	for k, a := range by {
		var s float64
		if a.wr > 0 {
			s += w.Words / float64(store.RRFK+a.wr)
		}
		if a.tr > 0 {
			s += w.Tri / float64(store.RRFK+a.tr)
		}
		if a.vr > 0 {
			s += w.Vec / float64(store.RRFK+a.vr)
		}
		out = append(out, Fused{Key: k, Score: s, BM25: a.bm})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.BM25 != b.BM25 {
			return a.BM25 > b.BM25
		}
		return a.Key < b.Key
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

type VectorSettings struct {
	Pool       Pool     `json:"pool"`
	LexRegular bool     `json:"lex_regular"`
	Reg        KindGate `json:"reg"`
	Log        KindGate `json:"log"`
	Weights    Weights  `json:"weights"`
	Depth      int      `json:"depth"`
	PoolSize   int      `json:"pool_size"`

	PointerTake int `json:"pointer_take"`
}

func DefaultVectorSettings() VectorSettings {
	return VectorSettings{
		Reg:      KindGate{Take: DefaultLimit, MinCos: 0.5, MinMargin: NoMargin},
		Log:      OffGate(),
		Weights:  DefaultWeights(),
		Depth:    DefaultVecDepth,
		PoolSize: DefaultPool,

		PointerTake: DefaultPointerTake,
	}
}

func (v VectorSettings) Enabled() bool { return v.Pool != PoolNone }

func (v VectorSettings) Build(ix *Index, scored []VHit, words, tri []store.ListHit) []VHit {
	n := v.PoolSize
	reg, _ := splitKinds(scored)
	switch v.Pool {
	case PoolReg:
		return dedupeFiles(reg, n)
	case PoolAll:
		return dedupeFiles(scored, n)
	case PoolFusedReg, PoolFusedAll:
		var vkeys []string
		limit := n
		if v.Pool == PoolFusedReg {
			for _, h := range reg[:min(v.Depth, len(reg))] {
				vkeys = append(vkeys, h.Key)
			}
		} else {
			for _, h := range dedupeFiles(scored, v.Depth) {
				vkeys = append(vkeys, h.Key)
			}
			limit = math.MaxInt
		}
		cos := make(map[string]float64, len(scored))
		for _, h := range scored {
			cos[h.Key] = h.Cos
		}
		fused := Fuse(words, tri, vkeys, v.Weights, limit)
		list := make([]VHit, len(fused))
		for i, f := range fused {
			list[i] = VHit{Key: f.Key, File: ix.fileOf(f.Key), Cos: cos[f.Key], Log: ix.isLog(f.Key)}
		}
		return dedupeFiles(list, n)
	}
	return nil
}

func (ix *Index) isLog(key string) bool {
	i, ok := ix.by[key]
	return ok && ix.Entries[i].Log
}

type Selection struct {
	Reg []VHit `json:"reg"`
	Log []VHit `json:"log"`
}

func (v VectorSettings) Select(ix *Index, scored []VHit, words, tri []store.ListHit) Selection {
	return v.Gate(v.Build(ix, scored, words, tri))
}

func (v VectorSettings) Gate(built []VHit) Selection {
	reg, logs := splitKinds(built)
	return Selection{Reg: v.Reg.Apply(reg), Log: v.Log.Apply(logs)}
}

func (v VectorSettings) PointersConfigured() bool {
	return v.Reg.PointerCos > 0 || v.Log.PointerCos > 0
}

func (v VectorSettings) PointerSelect(built []VHit, skip func(VHit) bool) Selection {
	reg, logs := splitKinds(built)
	return Selection{Reg: v.Reg.Pointers(reg, v.PointerTake, skip), Log: v.Log.Pointers(logs, v.PointerTake, skip)}
}

const (
	GatePointer  = "pointer"
	GatePass     = "pass"
	GateOff      = "off"
	GateBelowCos = "below-cos"
	GateMargin   = "margin"
	GateTake     = "take"
)

func (g KindGate) Judge(list []VHit) []string {
	out := make([]string, len(list))
	fill := func(v string) []string {
		for i := range out {
			out[i] = v
		}
		return out
	}
	switch {
	case len(list) == 0:
		return out
	case g.Take <= 0:
		return fill(GateOff)
	case list[0].Cos < g.MinCos:
		return fill(GateBelowCos)
	}
	if g.MinMargin > NoMargin {
		second := 0.0
		if len(list) > 1 {
			second = list[1].Cos
		}
		if list[0].Cos-second < g.MinMargin {
			return fill(GateMargin)
		}
	}
	for i, h := range list {
		switch {
		case i >= g.Take:
			out[i] = GateTake
		case h.Cos < g.MinCos:
			out[i] = GateBelowCos
		default:
			out[i] = GatePass
		}
	}
	return out
}

const ExpiredCandidates = 2

func SplitExpired(list []VHit) (live, expired []VHit) {
	for _, h := range list {
		if h.Expired {
			expired = append(expired, h)
		} else {
			live = append(live, h)
		}
	}
	return live, expired
}

func (v VectorSettings) pointerFloor() float64 {
	if v.Reg.PointerCos > 0 {
		return v.Reg.PointerCos
	}
	return v.Reg.MinCos
}

func (v VectorSettings) ExpiredNear(pool, expired []VHit, n int, skip func(VHit) bool) []VHit {
	floor, found := v.pointerFloor(), false
	for _, h := range pool {
		if !h.Log && h.Cos > 0 && (!found || h.Cos < floor) {
			floor, found = h.Cos, true
		}
	}
	var out []VHit
	for _, h := range expired {
		if len(out) == n {
			break
		}
		if !h.Log && h.Cos >= floor && (skip == nil || !skip(h)) {
			out = append(out, h)
		}
	}
	return out
}

func (v VectorSettings) ExpiredPointers(expired []VHit, skip func(VHit) bool) []VHit {
	if v.Reg.PointerCos <= 0 || v.PointerTake <= 0 {
		return nil
	}
	var out []VHit
	for _, h := range expired {
		if len(out) == ExpiredCandidates {
			break
		}
		if h.Log || h.Cos < v.Reg.PointerCos || skip != nil && skip(h) {
			continue
		}
		out = append(out, h)
	}
	return out
}

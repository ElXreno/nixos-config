package recall

import (
	"math"
	"slices"
	"strings"
	"testing"

	"mem/internal/store"
)

func reranked(pairs ...any) []Reranked {
	var out []Reranked
	for i := 0; i < len(pairs); i += 2 {
		key := pairs[i].(string)
		out = append(out, Reranked{VHit: VHit{Key: key, File: key, Cos: 0.5, Log: strings.HasPrefix(key, "log")}, Rerank: pairs[i+1].(float64)})
	}
	return out
}

func liveSettings() VectorSettings {
	v := DefaultVectorSettings()
	v.Pool = PoolReg
	v.Reg.MinCos, v.Reg.PointerCos = 0.65, 0.5
	return v
}

func TestRerankTierBoundaries(t *testing.T) {
	v := liveSettings()
	g := DefaultRerankGate()
	list := reranked(
		"at-text", 0.99,
		"just-below-text", math.Nextafter(0.99, 0),
		"at-pointer", 0.9,
		"just-below-pointer", math.Nextafter(0.9, 0),
		"one", 1.0,
		"zero", 0.0,
	)
	strong := keysOf(v.RerankStrong(g, list).Reg)
	if !slices.Equal(strong, []string{"one", "at-text"}) {
		t.Errorf("strong %v", strong)
	}
	skip := func(h VHit) bool { return slices.Contains(strong, h.Key) }
	ptrs := keysOf(v.RerankPointers(g, list, skip).Reg)
	if !slices.Equal(ptrs, []string{"just-below-text", "at-pointer"}) {
		t.Errorf("pointers %v", ptrs)
	}
}

func TestRerankTakeAndOverflow(t *testing.T) {
	v := liveSettings()
	g := DefaultRerankGate()
	list := reranked("a", 0.991, "b", 0.999, "c", 0.995, "d", 0.993, "e", 0.992, "f", 0.95, "g", 0.94, "h", 0.93)
	strong := keysOf(v.RerankStrong(g, list).Reg)
	if !slices.Equal(strong, []string{"b", "c", "d", "e"}) {
		t.Errorf("strong %v, want the four best in score order", strong)
	}
	skip := func(h VHit) bool { return slices.Contains(strong, h.Key) }
	if got := keysOf(v.RerankPointers(g, list, skip).Reg); !slices.Equal(got, []string{"a", "f", "g"}) {
		t.Errorf("pointers %v: a strong-tier record past the take is a pointer, then pointer-take caps", got)
	}
	g.Take = 0
	if got := v.RerankStrong(g, list).Reg; len(got) != 0 {
		t.Errorf("take 0: %v", keysOf(got))
	}
	g = DefaultRerankGate()
	g.Pointer = 0
	if got := v.RerankPointers(g, list, nil).Reg; len(got) != 0 {
		t.Errorf("pointer 0 is off: %v", keysOf(got))
	}
	v.PointerTake = 0
	if got := v.RerankPointers(DefaultRerankGate(), list, nil).Reg; len(got) != 0 {
		t.Errorf("pointer-take 0 is off: %v", keysOf(got))
	}
}

func TestRerankTiesKeepThePoolOrder(t *testing.T) {
	v := liveSettings()
	list := reranked("first", 0.995, "second", 0.999, "third", 0.995)
	if got := keysOf(v.RerankStrong(DefaultRerankGate(), list).Reg); !slices.Equal(got, []string{"second", "first", "third"}) {
		t.Errorf("strong %v", got)
	}
	if list[0].Key != "first" {
		t.Error("the selection reordered its input")
	}
}

func TestRerankKindsFollowTheKindGates(t *testing.T) {
	list := reranked("reg", 0.999, "log-a", 0.998, "log-b", 0.95)
	v := liveSettings()
	v.Pool = PoolAll
	if s := v.RerankStrong(DefaultRerankGate(), list); !slices.Equal(keysOf(s.Reg), []string{"reg"}) || len(s.Log) != 0 {
		t.Errorf("log take 0 keeps logs out: %+v", s)
	}
	if p := v.RerankPointers(DefaultRerankGate(), list, nil); len(p.Log) != 0 {
		t.Errorf("log pointers without a log gate: %+v", p)
	}
	v.Log.Take = 1
	s := v.RerankStrong(DefaultRerankGate(), list)
	if !slices.Equal(keysOf(s.Log), []string{"log-a"}) {
		t.Errorf("log strong %+v", s)
	}
	skip := func(h VHit) bool { return h.Key == "reg" || h.Key == "log-a" }
	if p := v.RerankPointers(DefaultRerankGate(), list, skip); !slices.Equal(keysOf(p.Log), []string{"log-b"}) || len(p.Reg) != 0 {
		t.Errorf("log pointers %+v", p)
	}
	v = liveSettings()
	v.LexRegular = true
	if s := v.RerankStrong(DefaultRerankGate(), list); len(s.Reg) != 0 {
		t.Errorf("lex-regular leaves regular strong records to the lexical gate: %+v", s)
	}
	v = liveSettings()
	v.Reg.Take, v.Reg.PointerCos = 0, 0
	if s, p := v.RerankStrong(DefaultRerankGate(), list), v.RerankPointers(DefaultRerankGate(), list, nil); len(s.Reg)+len(p.Reg) != 0 {
		t.Errorf("a kind switched off stays off: %+v %+v", s, p)
	}
}

func TestRerankCandidates(t *testing.T) {
	built := []VHit{{Key: "r1"}, {Key: "l1", Log: true}, {Key: "r2"}, {Key: "r3"}, {Key: "l2", Log: true}}
	v := liveSettings()
	g := DefaultRerankGate()
	if got := keysOf(v.RerankCandidates(built, g, 12)); !slices.Equal(got, []string{"r1", "r2", "r3"}) {
		t.Errorf("logs are no candidates while the log gate is off: %v", got)
	}
	if got := keysOf(v.RerankCandidates(built, g, 2)); !slices.Equal(got, []string{"r1", "r2"}) {
		t.Errorf("cap: %v", got)
	}
	v.Log.Take = 1
	if got := keysOf(v.RerankCandidates(built, g, 4)); !slices.Equal(got, []string{"r1", "l1", "r2", "r3"}) {
		t.Errorf("with logs: %v", got)
	}
}

func TestRerankDoc(t *testing.T) {
	r := store.Record{Key: "k", Title: "Заголовок", Body: strings.Repeat("ж", 900)}
	doc := RerankDoc(r, 800)
	head, body, ok := strings.Cut(doc, "\n")
	if !ok || head != "Заголовок" || body != strings.Repeat("ж", 800) {
		t.Errorf("doc %q...", doc[:40])
	}
	if got := RerankDoc(r, 0); got != "Заголовок\n"+r.Body {
		t.Error("0 keeps the whole body")
	}
	if got := RerankDoc(store.Record{Body: "only a body"}, 800); got != "only a body" {
		t.Errorf("no title: %q", got)
	}
	if got := RerankDoc(store.Record{Title: "t", Body: "short"}, 800); got != "t\nshort" {
		t.Errorf("short body: %q", got)
	}
}

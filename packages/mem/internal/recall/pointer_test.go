package recall

import (
	"slices"
	"strings"
	"testing"

	"mem/internal/store"
)

func rec(key, title, body string) store.Record {
	return store.Record{Key: key, Kind: store.Fact, Title: title, Body: body}
}

func TestPointersTierBoundaries(t *testing.T) {
	g := KindGate{Take: 4, MinCos: 0.65, MinMargin: NoMargin, PointerCos: 0.5}
	list := []VHit{
		{Key: "above", Cos: 0.70},
		{Key: "at-strong", Cos: 0.65},
		{Key: "just-below-strong", Cos: 0.6499999},
		{Key: "mid", Cos: 0.55},
		{Key: "at-pointer", Cos: 0.5},
		{Key: "just-below-pointer", Cos: 0.4999999},
	}
	got := keysOf(g.Pointers(list, 10, nil))
	want := []string{"just-below-strong", "mid", "at-pointer"}
	if !slices.Equal(got, want) {
		t.Errorf("pointers %v, want %v", got, want)
	}
	strong := keysOf(g.Apply(list))
	if !slices.Equal(strong, []string{"above", "at-strong"}) {
		t.Errorf("strong %v", strong)
	}
}

func TestPointersOffAndLimits(t *testing.T) {
	list := []VHit{{Key: "a", Cos: 0.6}, {Key: "b", Cos: 0.55}, {Key: "c", Cos: 0.52}, {Key: "d", Cos: 0.51}}
	off := KindGate{Take: 4, MinCos: 0.65, MinMargin: NoMargin}
	if got := off.Pointers(list, 3, nil); len(got) != 0 {
		t.Errorf("zero pointer cos is off, got %v", keysOf(got))
	}
	on := KindGate{Take: 4, MinCos: 0.65, MinMargin: NoMargin, PointerCos: 0.5}
	if got := keysOf(on.Pointers(list, 3, nil)); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("take 3: %v", got)
	}
	if got := on.Pointers(list, 0, nil); len(got) != 0 {
		t.Errorf("take 0: %v", keysOf(got))
	}
	skip := func(h VHit) bool { return h.Key == "a" || h.Key == "c" }
	if got := keysOf(on.Pointers(list, 2, skip)); !slices.Equal(got, []string{"b", "d"}) {
		t.Errorf("skip applies before take: %v", got)
	}
	inverted := KindGate{Take: 4, MinCos: 0.4, MinMargin: NoMargin, PointerCos: 0.5}
	if got := inverted.Pointers(list, 3, nil); len(got) != 0 {
		t.Errorf("pointer cos above the strong floor leaves an empty band: %v", keysOf(got))
	}
}

func TestPointerSelectSplitsKinds(t *testing.T) {
	v := DefaultVectorSettings()
	v.Reg = KindGate{Take: 4, MinCos: 0.65, MinMargin: NoMargin, PointerCos: 0.5}
	v.Log = KindGate{Take: 2, MinCos: 0.6, MinMargin: NoMargin, PointerCos: 0.45}
	v.PointerTake = 1
	built := []VHit{
		{Key: "r1", Cos: 0.8}, {Key: "r2", Cos: 0.55}, {Key: "r3", Cos: 0.52},
		{Key: "l1", Cos: 0.62, Log: true}, {Key: "l2", Cos: 0.5, Log: true}, {Key: "l3", Cos: 0.46, Log: true},
	}
	sel := v.PointerSelect(built, nil)
	if got := keysOf(sel.Reg); !slices.Equal(got, []string{"r2"}) {
		t.Errorf("reg pointers %v", got)
	}
	if got := keysOf(sel.Log); !slices.Equal(got, []string{"l2"}) {
		t.Errorf("log pointers %v", got)
	}
	v.PointerTake = 3
	if got := keysOf(v.PointerSelect(built, nil).Log); !slices.Equal(got, []string{"l2", "l3"}) {
		t.Errorf("log pointers with take 3: %v", got)
	}
}

func TestDefaultPointerSettings(t *testing.T) {
	v := DefaultVectorSettings()
	if v.PointerTake != 3 || v.Reg.PointerCos != 0 || v.Log.PointerCos != 0 {
		t.Errorf("defaults %+v", v)
	}
}

func TestRenderPointer(t *testing.T) {
	r := rec("k1", "A title.", "body line")
	if got, want := RenderPointer(r), "- `k1` "+Tag(r)+": A title\n"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
	r.Title = ""
	r.Body = "first line of the body\nsecond"
	if got, want := RenderPointer(r), "- `k1` "+Tag(r)+": first line of the body\n"; got != want {
		t.Errorf("untitled %q, want %q", got, want)
	}
}

func TestPackPointers(t *testing.T) {
	a, b, c := rec("pa", "Alpha", "x"), rec("pb", "Beta", "y"), rec("pc", "Gamma", "z")
	lines := []string{RenderPointer(a), RenderPointer(b), RenderPointer(c)}
	text := strings.TrimRight(PointerHeader+lines[0]+lines[1]+lines[2], "\n")
	p := PackPointers([]store.Record{a, b, c}, runes(text))
	if p.Text != text || len(p.Keys) != 3 || len(p.Dropped) != 0 {
		t.Errorf("all fit: %+v", p)
	}
	p = PackPointers([]store.Record{a, b, c}, runes(text)-1)
	if len(p.Keys) != 2 || !slices.Equal(p.Dropped, []string{"pc"}) {
		t.Errorf("one short: %+v", p)
	}
	p = PackPointers([]store.Record{a, b, c}, runes(PointerHeader)+runes(lines[0])-1)
	if len(p.Keys) != 1 {
		t.Errorf("first line fits without its newline: %+v", p)
	}
	p = PackPointers([]store.Record{a, b, c}, runes(PointerHeader)+runes(lines[0])-2)
	if p.Text != "" || len(p.Keys) != 0 || len(p.Dropped) != 3 {
		t.Errorf("none fit: %+v", p)
	}
	if p = PackPointers(nil, 1000); p.Text != "" {
		t.Errorf("empty: %+v", p)
	}
}

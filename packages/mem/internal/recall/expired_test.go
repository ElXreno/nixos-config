package recall

import (
	"testing"
	"time"

	"mem/internal/store"
)

func joinedKeys(list []VHit) string {
	out := ""
	for _, h := range list {
		out += h.Key + " "
	}
	return out
}

func TestExpiredRendering(t *testing.T) {
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	cur := store.Record{Key: "k", Kind: store.State, Status: store.Current, Title: "Board state", UpdatedAt: at}
	exp := cur
	exp.Status, exp.ValidTo = store.Expired, at.Add(48*time.Hour)
	if got := Tag(cur); got != "[state, as of 2026-11-05]" {
		t.Errorf("current tag %q", got)
	}
	if got := Tag(exp); got != "[state, EXPIRED since 2026-11-07]" {
		t.Errorf("expired tag %q", got)
	}
	if got := RenderPointer(cur); got != "- `k` [state, as of 2026-11-05]: Board state\n" {
		t.Errorf("current pointer %q", got)
	}
	if got := RenderPointer(exp); got != "- `k` [state, EXPIRED since 2026-11-07]: Board state (unverified: check it, then `mem verify` or `mem retire`)\n" {
		t.Errorf("expired pointer %q", got)
	}
}

func TestExpiredSelection(t *testing.T) {
	vs := DefaultVectorSettings()
	vs.Pool, vs.Reg.MinCos, vs.Reg.PointerCos = PoolReg, 0.7, 0.5
	scored := []VHit{{Key: "a", Cos: 0.9}, {Key: "x", Cos: 0.85, Expired: true}, {Key: "b", Cos: 0.6}, {Key: "y", Cos: 0.55, Expired: true},
		{Key: "z", Cos: 0.4, Expired: true}, {Key: "l", Cos: 0.8, Expired: true, Log: true}}
	live, expired := SplitExpired(scored)
	if joinedKeys(live) != "a b " || joinedKeys(expired) != "x y z l " {
		t.Fatalf("split: live %q expired %q", joinedKeys(live), joinedKeys(expired))
	}
	if got := joinedKeys(vs.ExpiredPointers(expired, nil)); got != "x y " {
		t.Errorf("pointers %q", got)
	}
	one := vs
	one.PointerTake = 1
	if got := joinedKeys(one.ExpiredPointers(expired, nil)); got != "x y " {
		t.Errorf("pointers with a pointer take of 1 %q", got)
	}
	if got := joinedKeys(vs.ExpiredPointers(expired, func(h VHit) bool { return h.Key == "x" })); got != "y " {
		t.Errorf("pointers with a seen key %q", got)
	}
	off := vs
	off.Reg.PointerCos = 0
	if got := off.ExpiredPointers(expired, nil); got != nil {
		t.Errorf("pointers without a pointer tier %v", got)
	}
	none := vs
	none.PointerTake = 0
	if got := none.ExpiredPointers(expired, nil); got != nil {
		t.Errorf("pointers with a pointer take of 0 %v", got)
	}
	if got := joinedKeys(vs.ExpiredNear(live, expired, 2, nil)); got != "x " {
		t.Errorf("near a pool whose floor is 0.6: %q", got)
	}
	if got := joinedKeys(vs.ExpiredNear([]VHit{{Key: "c", Cos: 0.1}}, expired, 2, nil)); got != "x y " {
		t.Errorf("near a pool whose floor is 0.1: %q", got)
	}
	if got := joinedKeys(vs.ExpiredNear(nil, expired, 5, nil)); got != "x y " {
		t.Errorf("near an empty pool: %q", got)
	}

	g := DefaultRerankGate()
	list := []Reranked{{VHit: VHit{Key: "x", Expired: true}, Rerank: 0.999}, {VHit: VHit{Key: "a"}, Rerank: 0.995}, {VHit: VHit{Key: "y", Expired: true}, Rerank: 0.5}}
	if sel := vs.RerankStrong(g, list); joinedKeys(sel.Reg) != "a " {
		t.Errorf("strong %q", joinedKeys(sel.Reg))
	}
	if sel := vs.RerankPointers(g, list, func(h VHit) bool { return h.Key == "a" }); joinedKeys(sel.Reg) != "x " {
		t.Errorf("pointers %q", joinedKeys(sel.Reg))
	}
}

func TestRerankPointersListExpiredAfterTheCurrentOnes(t *testing.T) {
	vs := DefaultVectorSettings()
	vs.Pool, vs.Reg.MinCos, vs.Reg.PointerCos = PoolReg, 0.7, 0.5
	g := DefaultRerankGate()
	list := []Reranked{
		{VHit: VHit{Key: "a"}, Rerank: 0.95},
		{VHit: VHit{Key: "b"}, Rerank: 0.94},
		{VHit: VHit{Key: "c"}, Rerank: 0.93},
		{VHit: VHit{Key: "d"}, Rerank: 0.92},
		{VHit: VHit{Key: "x", Expired: true}, Rerank: 0.96},
		{VHit: VHit{Key: "y", Expired: true}, Rerank: 0.97},
		{VHit: VHit{Key: "w", Expired: true}, Rerank: 0.98},
		{VHit: VHit{Key: "v", Expired: true}, Rerank: 0.5},
	}
	if vs.PointerTake != 3 {
		t.Fatalf("pointer take %d", vs.PointerTake)
	}
	if got := joinedKeys(vs.RerankPointers(g, list, nil).Reg); got != "a b c w y " {
		t.Errorf("pointers %q, want the three current records, then the two best expired ones", got)
	}
	if got := joinedKeys(vs.RerankPointers(g, list, func(h VHit) bool { return h.Key == "w" || h.Key == "a" }).Reg); got != "b c d y x " {
		t.Errorf("pointers with seen keys %q", got)
	}
	one := vs
	one.PointerTake = 1
	if got := joinedKeys(one.RerankPointers(g, list, nil).Reg); got != "a w y " {
		t.Errorf("pointers with a pointer take of 1 %q", got)
	}
	none := vs
	none.PointerTake = 0
	if sel := none.RerankPointers(g, list, nil); len(sel.Reg)+len(sel.Log) != 0 {
		t.Errorf("pointers with a pointer take of 0 %+v", sel)
	}
}

func TestExpiredNearSkipsShownPointers(t *testing.T) {
	vs := DefaultVectorSettings()
	vs.Pool, vs.Reg.MinCos, vs.Reg.PointerCos = PoolReg, 0.7, 0.5
	expired := []VHit{{Key: "x", Cos: 0.85, Expired: true}, {Key: "y", Cos: 0.80, Expired: true}, {Key: "z", Cos: 0.75, Expired: true}}
	pool := []VHit{{Key: "a", Cos: 0.6}}
	shown := func(h VHit) bool { return h.Key == "x" || h.Key == "y" }
	if got := joinedKeys(vs.ExpiredNear(pool, expired, ExpiredCandidates, shown)); got != "z " {
		t.Errorf("near with x and y shown %q, want z", got)
	}
}

func TestExpiredNearIgnoresFusedHitsWithoutVector(t *testing.T) {
	ix := NewIndex([]Entry{
		{Key: "a", File: "a", Vec: unitVec(1, 0, 0)},
		{Key: "x", File: "x", Expired: true, Vec: unitVec(0.05, 0.99875, 0)},
	})
	scored, err := ix.Score(unitVec(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	live, expired := SplitExpired(scored)
	vs := DefaultVectorSettings()
	vs.Pool, vs.Reg.MinCos, vs.Reg.PointerCos, vs.PointerTake = PoolFusedReg, 0.65, 0.5, 3
	words := []store.ListHit{{Key: "novec", Rank: 1, Score: 3}}
	built := vs.Build(ix, live, words, nil)
	cands := vs.RerankCandidates(built, DefaultRerankGate(), DefaultRerankCandidates)
	if got := joinedKeys(vs.ExpiredNear(cands, expired, ExpiredCandidates, nil)); got != "" {
		t.Errorf("expired %q at cos %.3f sent to the reranker: the floor came from a key without a vector", got, expired[0].Cos)
	}
	if got := joinedKeys(vs.ExpiredNear([]VHit{{Key: "novec"}}, expired, ExpiredCandidates, nil)); got != "" {
		t.Errorf("a pool without a positive cosine falls back to the pointer floor, yet %q passed", got)
	}
	near := []VHit{{Key: "x", Cos: 0.55, Expired: true}, {Key: "y", Cos: 0.45, Expired: true}}
	if got := joinedKeys(vs.ExpiredNear([]VHit{{Key: "novec"}}, near, ExpiredCandidates, nil)); got != "x " {
		t.Errorf("near a pool without a positive cosine %q, want only x above the pointer cosine", got)
	}
}

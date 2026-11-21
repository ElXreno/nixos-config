package recall

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"mem/internal/store"
)

func keysOf(l []VHit) []string {
	out := make([]string, len(l))
	for i, h := range l {
		out[i] = h.Key
	}
	return out
}

func unitVec(x, y, z float32) []float32 {
	n := float32(math.Sqrt(float64(x*x + y*y + z*z)))
	return []float32{x / n, y / n, z / n}
}

func TestKindGateApply(t *testing.T) {
	list := []VHit{{Key: "a", Cos: 0.80}, {Key: "b", Cos: 0.74}, {Key: "c", Cos: 0.40}, {Key: "d", Cos: 0.39}}
	cases := []struct {
		name string
		g    KindGate
		in   []VHit
		want []string
	}{
		{"take zero is off", KindGate{MinMargin: NoMargin}, list, nil},
		{"empty list", KindGate{Take: 2, MinMargin: NoMargin}, nil, nil},
		{"top below the floor", KindGate{Take: 2, MinCos: 0.9, MinMargin: NoMargin}, list, nil},
		{"floor filters later items", KindGate{Take: 4, MinCos: 0.7, MinMargin: NoMargin}, list, []string{"a", "b"}},
		{"take counts positions", KindGate{Take: 3, MinCos: 0.5, MinMargin: NoMargin}, list, []string{"a", "b"}},
		{"take limits", KindGate{Take: 1, MinCos: 0.1, MinMargin: NoMargin}, list, []string{"a"}},
		{"margin too small", KindGate{Take: 2, MinCos: 0.1, MinMargin: 0.08}, list, nil},
		{"margin met", KindGate{Take: 2, MinCos: 0.1, MinMargin: 0.05}, list, []string{"a", "b"}},
		{"margin zero passes", KindGate{Take: 1, MinCos: 0.1, MinMargin: 0}, list, []string{"a"}},
		{"single item against second zero", KindGate{Take: 1, MinCos: 0.1, MinMargin: 0.9}, list[:1], nil},
		{"single item margin met", KindGate{Take: 1, MinCos: 0.1, MinMargin: 0.5}, list[:1], []string{"a"}},
	}
	for _, c := range cases {
		if got := keysOf(c.g.Apply(c.in)); !slices.Equal(got, c.want) && (len(got) != 0 || len(c.want) != 0) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func testIndex() *Index {
	return NewIndex([]Entry{
		{Key: "rule-a", File: "rule-a", Vec: unitVec(1, 0, 0)},
		{Key: "rule-b", File: "rule-b", Vec: unitVec(1, 1, 0)},
		{Key: "fact-c", File: "fact-c", Vec: unitVec(0, 1, 0)},
		{Key: "journal#1", File: "journal", Log: true, Vec: unitVec(1, 0.2, 0)},
		{Key: "journal#2", File: "journal", Log: true, Vec: unitVec(1, 0.4, 0)},
		{Key: "other#1", File: "other", Log: true, Vec: unitVec(0, 0, 1)},
	})
}

func TestScore(t *testing.T) {
	ix := testIndex()
	got, err := ix.Score(unitVec(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"rule-a", "journal#1", "journal#2", "rule-b", "fact-c", "other#1"}
	if !slices.Equal(keysOf(got), want) {
		t.Errorf("order %v, want %v", keysOf(got), want)
	}
	if math.Abs(got[0].Cos-1) > 1e-6 || got[0].Log || !got[1].Log {
		t.Errorf("hits %+v", got[:2])
	}
	tied := NewIndex([]Entry{{Key: "b", File: "b", Vec: []float32{1, 0}}, {Key: "a", File: "a", Vec: []float32{1, 0}}})
	got, _ = tied.Score([]float32{1, 0})
	if !slices.Equal(keysOf(got), []string{"a", "b"}) {
		t.Errorf("ties not broken by key: %v", keysOf(got))
	}
}

func TestScoreRejectsBadQueries(t *testing.T) {
	ix := testIndex()
	nan := float32(math.NaN())
	cases := []struct {
		name string
		q    []float32
		want error
	}{
		{"wrong dimension", []float32{1, 0}, ErrDimension},
		{"NaN", []float32{nan, 0, 0}, ErrQuery},
		{"infinity", []float32{float32(math.Inf(1)), 0, 0}, ErrQuery},
		{"zero", []float32{0, 0, 0}, ErrQuery},
		{"empty", nil, ErrQuery},
	}
	for _, c := range cases {
		if _, err := ix.Score(c.q); !errorsIs(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}

func TestPools(t *testing.T) {
	ix := testIndex()
	scored, err := ix.Score(unitVec(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	words := []store.ListHit{{Key: "fact-c", Rank: 1, Score: 5}, {Key: "rule-b", Rank: 2, Score: 4}, {Key: "no-vector", Rank: 3, Score: 3}}
	tri := []store.ListHit{{Key: "fact-c", Rank: 1, Score: 2}}
	base := DefaultVectorSettings()
	base.PoolSize = 3
	cases := []struct {
		pool Pool
		want []string
	}{
		{PoolReg, []string{"rule-a", "rule-b", "fact-c"}},
		{PoolAll, []string{"rule-a", "journal#1", "rule-b"}},
		{PoolFusedReg, []string{"fact-c", "rule-b", "rule-a"}},
		{PoolFusedAll, []string{"fact-c", "rule-b", "rule-a"}},
	}
	for _, c := range cases {
		v := base
		v.Pool = c.pool
		if got := keysOf(v.Build(ix, scored, words, tri)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.pool, got, c.want)
		}
	}
}

func TestFusedAllPoolHoldsOneChunkPerFile(t *testing.T) {
	ix := testIndex()
	scored, _ := ix.Score(unitVec(1, 0, 0))
	v := DefaultVectorSettings()
	v.Pool = PoolFusedAll
	v.PoolSize = 6
	got := keysOf(v.Build(ix, scored, nil, nil))
	want := []string{"rule-a", "journal#1", "rule-b", "fact-c", "other#1"}
	if !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

func TestFusedPoolKeepsKeysWithoutVectors(t *testing.T) {
	ix := testIndex()
	scored, _ := ix.Score(unitVec(1, 0, 0))
	words := []store.ListHit{{Key: "no-vector", Rank: 1, Score: 3}}
	v := DefaultVectorSettings()
	v.Pool = PoolFusedReg
	v.Weights.Vec = 0
	got := v.Build(ix, scored, words, nil)
	if len(got) != 1 || got[0].Key != "no-vector" || got[0].Cos != 0 || got[0].File != "no-vector" {
		t.Errorf("%+v", got)
	}
	v.Reg = KindGate{Take: 4, MinCos: 0.3, MinMargin: NoMargin}
	if sel := v.Select(ix, scored, words, nil); len(sel.Reg) != 0 {
		t.Errorf("a hit without a vector passed the cosine floor: %+v", sel.Reg)
	}
}

func TestSelectSplitsKindsAndGatesEach(t *testing.T) {
	ix := testIndex()
	scored, _ := ix.Score(unitVec(1, 0, 0))
	v := DefaultVectorSettings()
	v.Pool = PoolAll
	v.PoolSize = 6
	v.Reg = KindGate{Take: 2, MinCos: 0.6, MinMargin: NoMargin}
	v.Log = KindGate{Take: 2, MinCos: 0.9, MinMargin: NoMargin}
	sel := v.Select(ix, scored, nil, nil)
	if got := keysOf(sel.Reg); !slices.Equal(got, []string{"rule-a", "rule-b"}) {
		t.Errorf("regular %v", got)
	}
	if got := keysOf(sel.Log); !slices.Equal(got, []string{"journal#1"}) {
		t.Errorf("log %v (one chunk per file, floor 0.9)", got)
	}
	v.Pool = PoolReg
	if sel := v.Select(ix, scored, nil, nil); len(sel.Log) != 0 {
		t.Errorf("the reg pool injected logs: %v", keysOf(sel.Log))
	}
	v.Pool = PoolAll
	v.Log = OffGate()
	if sel := v.Select(ix, scored, nil, nil); len(sel.Log) != 0 {
		t.Errorf("a disabled log gate injected %v", keysOf(sel.Log))
	}
}

func TestFuseWeights(t *testing.T) {
	words := []store.ListHit{{Key: "w", Rank: 1, Score: 2}}
	tri := []store.ListHit{{Key: "t", Rank: 1, Score: 2}}
	vkeys := []string{"v"}
	order := func(w Weights) []string {
		var out []string
		for _, f := range Fuse(words, tri, vkeys, w, 10) {
			out = append(out, f.Key)
		}
		return out
	}
	if got := order(Weights{Words: 1, Tri: 0.3, Vec: 1}); got[2] != "t" {
		t.Errorf("default %v", got)
	}
	if got := order(Weights{Words: 1, Tri: 0.3, Vec: 3}); got[0] != "v" {
		t.Errorf("heavy vector %v", got)
	}
	if got := order(Weights{Words: 1, Tri: 0.3, Vec: 0}); slices.Contains(got, "v") && got[0] == "v" {
		t.Errorf("zero vector weight still ranks it first: %v", got)
	}
	if got := order(Weights{Words: 0, Tri: 0, Vec: 1}); got[0] != "v" {
		t.Errorf("vector only %v", got)
	}
	if n := len(Fuse(words, tri, vkeys, DefaultWeights(), 2)); n != 2 {
		t.Errorf("limit gave %d", n)
	}
}

func TestFileID(t *testing.T) {
	cases := []struct {
		key    string
		kind   store.Kind
		source string
		want   string
	}{
		{"feedback_x", store.Rule, "legacy:/home/u/mem/feedback_x.md", "feedback_x"},
		{"big_file#3", store.Log, "legacy:/home/u/mem/big_file.md", "big_file"},
		{"big_file#3", store.Log, "", "big_file"},
		{"plain#3", store.Rule, "", "plain#3"},
		{"plain", store.Fact, "", "plain"},
	}
	for _, c := range cases {
		if got := FileID(c.key, c.kind, c.source); got != c.want {
			t.Errorf("%s: %q, want %q", c.key, got, c.want)
		}
	}
}

func TestLoadIndex(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	recs := []store.Record{
		{Key: "rule-a", Kind: store.Rule, Scopes: []string{"global"}, Title: "A", Body: "alpha", Source: "legacy:/x/rule-a.md"},
		{Key: "journal#1", Kind: store.Log, Scopes: []string{"project:p"}, Body: "log chunk"},
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndex(ctx, s, "m", store.Query{}); err != ErrNoVectors {
		t.Errorf("empty store: %v", err)
	}
	var rows []store.VecRow
	for _, r := range recs {
		got, _ := s.Get(ctx, r.Key)
		rows = append(rows, store.VecRow{Key: r.Key, Vec: []float32{1, 0}, TextSHA: store.TextSHA(store.DocText("", got))})
	}
	if _, _, err := s.PutVectors(ctx, "m", "", rows); err != nil {
		t.Fatal(err)
	}
	ix, err := LoadIndex(ctx, s, "m", store.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if ix.Dim != 2 || len(ix.Entries) != 2 || ix.Entries[0].File != "journal" || !ix.Entries[0].Log || ix.Entries[1].File != "rule-a" {
		t.Errorf("%+v", ix)
	}
	ix, err = LoadIndex(ctx, s, "m", store.Query{ExcludeKinds: []store.Kind{store.Log}, Scopes: []string{"global"}})
	if err != nil || len(ix.Entries) != 1 {
		t.Errorf("filtered: %v %v", ix, err)
	}
}

func errorsIs(err, target error) bool { return err != nil && errors.Is(err, target) }

func TestJudgeAgreesWithApply(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for range 2000 {
		g := KindGate{Take: rng.Intn(4), MinCos: rng.Float64() * 0.8}
		if rng.Intn(2) == 0 {
			g.MinMargin = NoMargin
		} else {
			g.MinMargin = rng.Float64() * 0.2
		}
		list := make([]VHit, rng.Intn(6))
		cos := 1.0
		for i := range list {
			cos -= rng.Float64() * 0.25
			list[i] = VHit{Key: fmt.Sprintf("k%d", i), Cos: cos}
		}
		verdicts := g.Judge(list)
		if len(verdicts) != len(list) {
			t.Fatalf("%d verdicts for %d hits", len(verdicts), len(list))
		}
		var passed []string
		for i, v := range verdicts {
			if v == GatePass {
				passed = append(passed, list[i].Key)
			}
		}
		var want []string
		for _, h := range g.Apply(list) {
			want = append(want, h.Key)
		}
		if !slices.Equal(passed, want) {
			t.Fatalf("gate %+v list %+v: verdicts %v pass %v, Apply gives %v", g, list, verdicts, passed, want)
		}
	}
}

func TestJudgeNamesTheReason(t *testing.T) {
	list := []VHit{{Key: "a", Cos: 0.9}, {Key: "b", Cos: 0.8}, {Key: "c", Cos: 0.4}}
	cases := []struct {
		g    KindGate
		want []string
	}{
		{KindGate{Take: 0, MinMargin: NoMargin}, []string{GateOff, GateOff, GateOff}},
		{KindGate{Take: 5, MinCos: 0.95, MinMargin: NoMargin}, []string{GateBelowCos, GateBelowCos, GateBelowCos}},
		{KindGate{Take: 5, MinCos: 0.5, MinMargin: 0.3}, []string{GateMargin, GateMargin, GateMargin}},
		{KindGate{Take: 1, MinCos: 0.5, MinMargin: NoMargin}, []string{GatePass, GateTake, GateTake}},
		{KindGate{Take: 3, MinCos: 0.5, MinMargin: NoMargin}, []string{GatePass, GatePass, GateBelowCos}},
	}
	for i, c := range cases {
		if got := c.g.Judge(list); !slices.Equal(got, c.want) {
			t.Errorf("case %d: %v, want %v", i, got, c.want)
		}
	}
}

func TestGateIsSelectWithoutTheBuild(t *testing.T) {
	ix := NewIndex([]Entry{{Key: "a", File: "a", Vec: []float32{1, 0}}, {Key: "b", File: "b", Vec: []float32{0.8, 0.6}}, {Key: "log#1", File: "log", Log: true, Vec: []float32{0.9, 0.1}}})
	scored, err := ix.Score([]float32{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	v := DefaultVectorSettings()
	v.Pool = PoolAll
	v.Reg.MinCos, v.Log = 0.5, KindGate{Take: 1, MinCos: 0.5, MinMargin: NoMargin}
	if got, want := v.Gate(v.Build(ix, scored, nil, nil)), v.Select(ix, scored, nil, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("%+v vs %+v", got, want)
	}
}

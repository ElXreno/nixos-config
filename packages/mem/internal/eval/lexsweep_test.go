package eval

import (
	"fmt"
	"slices"
	"testing"

	"mem/internal/recall"
)

func TestLexSweepGridHoldsTheShippedGate(t *testing.T) {
	i := slices.Index(SweepGrid(), recall.DefaultGate())
	if i < 0 {
		t.Fatal("the shipped gate is not in the sweep grid")
	}
}

func TestLexSweepPicksMatchTheShippedGate(t *testing.T) {
	pf := newPoolFixture(t)
	ev := pf.ev
	ev.cfg.LexSweep = true
	grid := SweepGrid()
	def := slices.Index(grid, recall.DefaultGate())
	multi := false
	for _, q0 := range pf.qs {
		res, err := ev.one(t.Context(), q0, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.lexGate) != len(grid) {
			t.Fatalf("%d gate indexes, %d gates", len(res.lexGate), len(grid))
		}
		got := res.lexSets[res.lexGate[def]]
		if !slices.Equal(got, res.lexPick) {
			t.Errorf("sweep at the shipped gate picks %v, the hook picks %v", got, res.lexPick)
		}
		if len(res.lexSets) > 1 {
			multi = true
		}
		if len(res.lexSets) > len(grid) {
			t.Errorf("%d pick sets for %d gates", len(res.lexSets), len(grid))
		}
	}
	if !multi {
		t.Error("no prompt has two different pick sets across the grid, the test has no power")
	}
}

func TestLexSweepVariantIsCrossFitted(t *testing.T) {
	pf := newPoolFixture(t)
	ev := pf.ev
	ev.cfg.LexSweep = true
	ev.cfg.Folds, ev.cfg.FIRBudgets = 3, []float64{0.2, 1}
	var gold, neg []*query
	var gres, nres []qres
	for i, q0 := range pf.qs {
		q := &query{Prompt: q0.Prompt}
		q.Session = fmt.Sprint("sess", i%5)
		ev.qIndex[queryID(q)] = ev.qIndex[queryID(q0)]
		res, err := ev.one(t.Context(), q, true)
		if err != nil {
			t.Fatal(err)
		}
		if i >= 20 {
			neg, nres = append(neg, q), append(nres, res)
			continue
		}
		q.gold = []string{res.pools[PoolReg][i%3].slug}
		gold, gres = append(gold, q), append(gres, res)
	}
	lg, sets, err := ev.logsGate(pf.byKey, map[string]bool{}, gold, neg, gres, nres)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, r := range lg.Results {
		if r.Variant != lexSweepVariant {
			continue
		}
		seen++
		if r.Gold != len(gold) || r.Negatives != len(neg) || len(r.Folds) != 3 {
			t.Errorf("%+v", r)
		}
		for _, f := range r.Folds {
			if f.LexGate == nil || f.Lexical || f.Log.Take != 0 {
				t.Errorf("fold %d: lexical gate %v, log %+v", f.Fold, f.LexGate, f.Log)
			}
			if f.Feasible && f.TrainFIR > r.Budget+1e-9 {
				t.Errorf("fold %d: train FIR %v above budget %v", f.Fold, f.TrainFIR, r.Budget)
			}
		}
		if r.LogRecords != 0 {
			t.Errorf("log records %d in a regular-only variant", r.LogRecords)
		}
	}
	if seen != 2 {
		t.Fatalf("%d lex-sweep results, want one per budget", seen)
	}
	n := 0
	for _, s := range sets {
		if s.variant == lexSweepVariant {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d held-out row sets", n)
	}
}

package eval

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

func keysOf(l []vh) []string {
	var out []string
	for _, h := range l {
		out = append(out, h.key)
	}
	return out
}

func TestDedupeFilesKeepsBestChunkPerFile(t *testing.T) {
	list := []vh{
		{key: "saga#2", slug: "saga", cos: 0.9, log: true},
		{key: "saga#1", slug: "saga", cos: 0.8, log: true},
		{key: "note", slug: "note", cos: 0.7},
		{key: "saga#3", slug: "saga", cos: 0.6, log: true},
		{key: "diary#1", slug: "diary", cos: 0.5, log: true},
		{key: "native-a", cos: 0.4},
		{key: "native-b", cos: 0.3},
	}
	if got, want := keysOf(dedupeFiles(list, 10)), []string{"saga#2", "note", "diary#1", "native-a", "native-b"}; !slices.Equal(got, want) {
		t.Errorf("dedupe: %v, want %v", got, want)
	}
	if got, want := keysOf(dedupeFiles(list, 2)), []string{"saga#2", "note"}; !slices.Equal(got, want) {
		t.Errorf("dedupe n=2: %v, want %v", got, want)
	}
	if got := dedupeFiles(nil, 3); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
}

func TestSplitKindsKeepsOrder(t *testing.T) {
	list := []vh{{key: "a"}, {key: "l1", log: true}, {key: "b"}, {key: "l2", log: true}}
	reg, logs := splitKinds(list)
	if !slices.Equal(keysOf(reg), []string{"a", "b"}) || !slices.Equal(keysOf(logs), []string{"l1", "l2"}) {
		t.Errorf("reg %v logs %v", keysOf(reg), keysOf(logs))
	}
}

func TestKindGateApply(t *testing.T) {
	list := []vh{{key: "a", cos: 0.8}, {key: "b", cos: 0.7}, {key: "c", cos: 0.4}}
	for _, c := range []struct {
		g    KindGate
		want []string
	}{
		{KindGate{Take: 0, MinCos: 0, MinMargin: noMargin}, nil},
		{KindGate{Take: 1, MinCos: 0.5, MinMargin: noMargin}, []string{"a"}},
		{KindGate{Take: 4, MinCos: 0.3, MinMargin: noMargin}, []string{"a", "b", "c"}},
		{KindGate{Take: 2, MinCos: 0.3, MinMargin: noMargin}, []string{"a", "b"}},
		{KindGate{Take: 1, MinCos: 0.85, MinMargin: noMargin}, nil},
		{KindGate{Take: 1, MinCos: 0.5, MinMargin: 0.2}, nil},
		{KindGate{Take: 1, MinCos: 0.5, MinMargin: 0.1}, []string{"a"}},
	} {
		if got := keysOf(c.g.apply(list)); !slices.Equal(got, c.want) {
			t.Errorf("%+v: %v want %v", c.g, got, c.want)
		}
	}
	if got := (KindGate{Take: 1, MinCos: 0.5, MinMargin: 0.3}).apply([]vh{{key: "a", cos: 0.9}}); len(got) != 1 {
		t.Errorf("single candidate: %v", got)
	}
	if got := (KindGate{Take: 1, MinCos: 0.1, MinMargin: 0}).apply([]vh{{key: "a", cos: 0.2}, {key: "b", cos: 0.6}}); got != nil {
		t.Errorf("negative margin passes: %v", got)
	}
}

func TestKindGateApplyMatchesGateInject(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	for range 300 {
		var list []vh
		for i := range r.IntN(6) {
			list = append(list, vh{key: fmt.Sprint("k", i), cos: r.Float64()})
		}
		slices.SortFunc(list, func(a, b vh) int {
			if a.cos > b.cos {
				return -1
			}
			return 1
		})
		g := KindGate{Take: 1 + r.IntN(4), MinCos: r.Float64(), MinMargin: marginValues[r.IntN(len(marginValues))]}
		got, want := keysOf(g.apply(list)), gateInject(list, g.Take, g.MinCos, g.MinMargin)
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%+v on %v: %v vs gateInject %v", g, keysOf(list), got, want)
		}
	}
}

func TestFoldK(t *testing.T) {
	first, again := foldK("abc", 5), foldK("abc", 5)
	if first != again {
		t.Fatal("not deterministic")
	}
	count := make([]int, 5)
	for i := range 500 {
		f := foldK(fmt.Sprintf("session-%d", i), 5)
		if f < 0 || f >= 5 {
			t.Fatalf("fold %d", f)
		}
		count[f]++
	}
	for f, n := range count {
		if n < 60 || n > 140 {
			t.Errorf("fold %d holds %d of 500 sessions: %v", f, n, count)
		}
	}
	if foldK("x", 1) != 0 {
		t.Error("one fold")
	}
	same := 0
	for i := range 500 {
		s := fmt.Sprintf("session-%d", i)
		if foldK(s, 5)%2 == foldK(s, 2) {
			same++
		}
	}
	if same == 500 {
		t.Error("K=5 folds are the K=2 split in disguise")
	}
}

func regItems(p *promptInput) []string {
	var out []string
	for _, h := range (KindGate{Take: 1, MinCos: 0.5, MinMargin: noMargin}).apply(p.reg) {
		out = append(out, h.slug)
	}
	return out
}

func logItems(p *promptInput) []string {
	var out []string
	for _, h := range (KindGate{Take: 1, MinCos: 0.5, MinMargin: noMargin}).apply(p.logs) {
		out = append(out, h.slug)
	}
	return out
}

func offItems(*promptInput) []string { return nil }

func TestLayoutStatsAndCombine(t *testing.T) {
	gold := []promptInput{
		{fold: 0, goldLen: 2, gold: []string{"A", "B"}, reg: []vh{{key: "A", slug: "A", cos: 0.9}}, logs: []vh{{key: "B#1", slug: "B", cos: 0.7, log: true}}},
		{fold: 1, goldLen: 1, gold: []string{"C"}, reg: []vh{{key: "x", slug: "x", cos: 0.3}}, logs: []vh{{key: "C#2", slug: "C", cos: 0.8, log: true}}},
		{fold: 0, goldLen: 1, gold: []string{"D"}, reg: []vh{{key: "D", slug: "D", cos: 0.6}}},
	}
	neg := []promptInput{
		{fold: 0, reg: []vh{{key: "x", slug: "x", cos: 0.6}}, logs: []vh{{key: "y#1", slug: "y", cos: 0.6, log: true}}},
		{fold: 0, reg: []vh{{key: "x", slug: "x", cos: 0.4}}, logs: []vh{{key: "y#1", slug: "y", cos: 0.4, log: true}}},
		{fold: 1, logs: []vh{{key: "y#1", slug: "y", cos: 0.9, log: true}}},
	}
	l := newLayout(2, gold, neg)
	if !slices.Equal(l.goldN, []int{2, 1}) || !slices.Equal(l.negN, []int{2, 1}) {
		t.Fatalf("fold sizes gold %v neg %v", l.goldN, l.negN)
	}
	reg, logs := l.stats(regItems), l.stats(logItems)
	off := l.stats(offItems)
	check := func(name string, got []foldTotals, rec [2]float64, hit, neg [2]int) {
		t.Helper()
		if len(got) != 2 {
			t.Fatalf("%s: %d folds", name, len(got))
		}
		for f := range 2 {
			if math.Abs(got[f].rec-rec[f]) > 1e-12 || got[f].hit != hit[f] || got[f].neg != neg[f] {
				t.Errorf("%s fold %d: rec %v hit %d neg %d, want %v %d %d", name, f, got[f].rec, got[f].hit, got[f].neg, rec[f], hit[f], neg[f])
			}
			if got[f].goldN != l.goldN[f] || got[f].nN != l.negN[f] {
				t.Errorf("%s fold %d sizes %d %d", name, f, got[f].goldN, got[f].nN)
			}
		}
	}
	check("reg+off", l.combine(&reg, &off), [2]float64{1.5, 0}, [2]int{2, 0}, [2]int{1, 0})
	check("off+log", l.combine(&off, &logs), [2]float64{0.5, 1}, [2]int{1, 1}, [2]int{1, 1})
	check("reg+log", l.combine(&reg, &logs), [2]float64{2, 1}, [2]int{2, 1}, [2]int{1, 1})
	check("off+off", l.combine(&off, &off), [2]float64{0, 0}, [2]int{0, 0}, [2]int{0, 0})
}

func TestLayoutHandlesMoreThanSixtyFourPrompts(t *testing.T) {
	var neg []promptInput
	for i := range 200 {
		p := promptInput{fold: i % 3}
		if i%7 == 0 {
			p.reg = []vh{{key: "x", slug: "x", cos: 0.9}}
		}
		if i%5 == 0 {
			p.logs = []vh{{key: "y#1", slug: "y", cos: 0.9, log: true}}
		}
		neg = append(neg, p)
	}
	l := newLayout(3, nil, neg)
	reg, logs := l.stats(regItems), l.stats(logItems)
	got := l.combine(&reg, &logs)
	var want [3]int
	for i, p := range neg {
		if len(regItems(&p)) > 0 || len(logItems(&p)) > 0 {
			want[neg[i].fold]++
		}
	}
	for f := range 3 {
		if got[f].neg != want[f] || got[f].nN != l.negN[f] {
			t.Errorf("fold %d: %d injecting, want %d", f, got[f].neg, want[f])
		}
	}
}

func crossFitFixture() (*layout, []settingStats, []settingStats) {
	var gold, neg []promptInput
	for f := range 3 {
		gold = append(gold,
			promptInput{fold: f, goldLen: 1, gold: []string{"A"}, reg: []vh{{key: "A", slug: "A", cos: 0.6}}},
			promptInput{fold: f, goldLen: 1, gold: []string{"B"}, logs: []vh{{key: "B#1", slug: "B", cos: 0.7, log: true}}})
		neg = append(neg,
			promptInput{fold: f, reg: []vh{{key: "x", slug: "x", cos: 0.6}}},
			promptInput{fold: f, logs: []vh{{key: "y#1", slug: "y", cos: 0.8, log: true}}},
			promptInput{fold: f}, promptInput{fold: f})
	}
	l := newLayout(3, gold, neg)
	return l, []settingStats{l.stats(offItems), l.stats(regItems)}, []settingStats{l.stats(offItems), l.stats(logItems)}
}

func TestCrossFitPicks(t *testing.T) {
	l, reg, logs := crossFitFixture()
	picks := l.crossFit(reg, logs, []float64{0.1, 0.25, 0.5, -1}, 1)
	if len(picks) != 4 {
		t.Fatalf("budgets %d", len(picks))
	}
	want := []struct {
		reg, log int
		feasible bool
	}{{0, 0, true}, {0, 1, true}, {1, 1, true}, {0, 0, false}}
	for b, w := range want {
		if len(picks[b]) != 3 {
			t.Fatalf("budget %d: %d folds", b, len(picks[b]))
		}
		for f, p := range picks[b] {
			if p.Reg != w.reg || p.Log != w.log || p.Feasible != w.feasible {
				t.Errorf("budget %d fold %d: reg %d log %d feasible %v, want %+v", b, f, p.Reg, p.Log, p.Feasible, w)
			}
			if p.Train.goldN != 4 || p.Train.nN != 8 {
				t.Errorf("budget %d fold %d: train sizes %d %d, want 4 and 8", b, f, p.Train.goldN, p.Train.nN)
			}
		}
	}
	if p := picks[2][0]; math.Abs(p.Train.rec-4) > 1e-12 || p.Train.hit != 4 || p.Train.neg != 4 {
		t.Errorf("train totals of the budget-0.5 pick: %+v", p.Train)
	}
}

func TestCrossFitPicksIgnoreWorkerCount(t *testing.T) {
	l, reg, logs := crossFitFixture()
	a := l.crossFit(reg, logs, []float64{0.25, 0.5}, 1)
	b := l.crossFit(reg, logs, []float64{0.25, 0.5}, 7)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("1 worker %v, 7 workers %v", a, b)
	}
}

func TestCrossFitPicksSkipEmptyFolds(t *testing.T) {
	gold := []promptInput{{fold: 0, goldLen: 1, gold: []string{"A"}, reg: []vh{{key: "A", slug: "A", cos: 0.6}}}}
	neg := []promptInput{{fold: 2}}
	l := newLayout(5, gold, neg)
	picks := l.crossFit([]settingStats{l.stats(offItems), l.stats(regItems)}, []settingStats{l.stats(offItems)}, []float64{0.5}, 2)
	if len(picks) != 1 || len(picks[0]) != 5 {
		t.Fatalf("picks %v", picks)
	}
	if picks[0][1].Reg != 1 {
		t.Errorf("fold 1 is empty and trains on the rest, which holds the only gold: %+v", picks[0][1])
	}
}

func TestChunkKeysMapToFileSlug(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	var chunks []string
	for k, slug := range o.p.keySlug {
		if strings.HasPrefix(k, "project_saga#") {
			chunks = append(chunks, k)
			if slug != "project_saga" {
				t.Errorf("chunk %s maps to %q, want the file slug", k, slug)
			}
		}
	}
	if len(chunks) < 2 || !o.p.logSlug["project_saga"] {
		t.Fatalf("fixture must hold a chunked file: chunks %v, log slug %v", chunks, o.p.logSlug["project_saga"])
	}
	slices.Sort(chunks)
	only := []vh{{key: "x", slug: "x"}, {key: chunks[len(chunks)-1], slug: o.p.keySlug[chunks[len(chunks)-1]], log: true}}
	if sc := scoreList(only, []string{"project_saga"}); sc.best != 2 || sc.n != 1 {
		t.Errorf("a late chunk must count as the gold file: %+v", sc)
	}
}

func TestChunkStats(t *testing.T) {
	logSlug := map[string]bool{"saga": true}
	gold := []*query{
		{gold: []string{"saga"}}, {gold: []string{"saga"}}, {gold: []string{"saga"}}, {gold: []string{"note"}},
	}
	raw := [][]vh{
		{{key: "saga#1", slug: "saga", log: true}, {key: "saga#2", slug: "saga", log: true}, {key: "x", slug: "x"}, {key: "saga#3", slug: "saga", log: true}},
		{{key: "z", slug: "z"}, {key: "saga#4", slug: "saga", log: true}},
		{{key: "z", slug: "z"}},
		{{key: "note", slug: "note"}},
	}
	neg := [][]vh{
		{{key: "a#1", slug: "a", log: true}, {key: "a#2", slug: "a", log: true}},
		{{key: "b#1", slug: "b", log: true}},
		{{key: "c", slug: "c"}},
	}
	st := chunkStats(gold, raw, neg, logSlug)
	want := ChunkStats{GoldLogPrompts: 3, GoldInPool: 2, GoldMulti2: 1, GoldMulti3: 1, GoldMeanChunks: 2,
		TopLogGold: 1, TopLogRepeatGold: 1, TopLogNeg: 2, TopLogRepeatNeg: 1}
	if st != want {
		t.Errorf("chunk stats %+v\nwant %+v", st, want)
	}
}

func readTSV(t *testing.T, path string) (map[string]int, [][]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	col := map[string]int{}
	for i, n := range strings.Split(lines[0], "\t") {
		col[n] = i
	}
	var rows [][]string
	for _, l := range lines[1:] {
		rows = append(rows, strings.Split(l, "\t"))
	}
	return col, rows
}

func TestLogsGateFlagValidation(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	out := filepath.Join(fx.root, "bad")
	for name, c := range map[string]struct {
		args []string
		msg  string
	}{
		"without vectors": {[]string{"--logs-gate", "--out", out}, "--logs-gate needs --vectors"},
		"one fold":        {[]string{"--vectors", o.dir, "--logs-gate", "--folds", "1", "--out", out}, "--folds"},
		"bad budget":      {[]string{"--vectors", o.dir, "--logs-gate", "--fir-budgets", "0.1,x", "--out", out}, "--fir-budgets"},
		"budget above 1":  {[]string{"--vectors", o.dir, "--logs-gate", "--fir-budgets", "1.5", "--out", out}, "--fir-budgets"},
		"bad pool":        {[]string{"--vectors", o.dir, "--logs-gate", "--pools", "reg,nope", "--out", out}, "--pools"},
		"folds w/o gate":  {[]string{"--vectors", o.dir, "--folds", "3", "--out", out}, "only with --logs-gate"},
		"no lexical ref":  {[]string{"--vectors", o.dir, "--logs-gate", "--lexical-ref=false", "--out", out}, "--lexical-ref"},
		"sweep w/o gate":  {[]string{"--vectors", o.dir, "--lex-sweep", "--out", out}, "only with --logs-gate"},
	} {
		fx.stderr.Reset()
		if code := Run(fx.args(c.args...), fx.e); code != 2 || !strings.Contains(fx.stderr.String(), c.msg) {
			t.Errorf("%s: exit %d, want 2 with %q: %s", name, code, c.msg, fx.stderr.String())
		}
	}
	fx.stderr.Reset()
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--logs-gate", "--folds", "3", "--fir-budgets", "0.2", "--pools", "reg,all", "--out", out), fx.e); code != 0 {
		t.Errorf("valid flags: exit %d: %s", code, fx.stderr.String())
	}
}

func distinctFoldK(t *testing.T, sessions []string) int {
	t.Helper()
	for k := 2; k <= 6; k++ {
		seen := map[int]bool{}
		for _, s := range sessions {
			seen[foldK(s, k)] = true
		}
		if len(seen) >= 2 {
			return k
		}
	}
	t.Fatalf("no K splits the sessions %v", sessions)
	return 0
}

func TestEvaluateVectorsLogsGate(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	var sessions []string
	for _, q := range o.p.qs {
		if !slices.Contains(sessions, q.Session) {
			sessions = append(sessions, q.Session)
		}
	}
	k := distinctFoldK(t, sessions)
	out := filepath.Join(fx.root, "lgout")
	args := fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out, "--logs-gate", "--folds", strconv.Itoa(k), "--fir-budgets", "0.5,1")
	if code := Run(args, fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
	for _, leak := range []string{longMarker, "git push the agent-guard", "zfs readahead batch patches", "Never send 0xDF"} {
		if strings.Contains(fx.stdout.String(), leak) {
			t.Errorf("stdout leaks text %q", leak)
		}
	}
	for _, name := range []string{"logs-gate-summary.json", "logs-gate-summary.tsv", "logs-gate-picks.tsv", "logs-gate-heldout.tsv"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", name, info, err)
			continue
		}
		data, _ := os.ReadFile(filepath.Join(out, name))
		for _, leak := range []string{longMarker, "zfs readahead batch patches", "Never send 0xDF"} {
			if strings.Contains(string(data), leak) {
				t.Errorf("%s leaks record or prompt text", name)
			}
		}
	}
	sum := loadVectorSummary(t, out)
	lg := sum.LogsGate
	if lg == nil || lg.K != k || len(lg.Budgets) != 2 {
		t.Fatalf("logs gate summary %+v", lg)
	}
	wantVariants := []string{"reg", "fused-reg", "all", "fused-all", "lex+all", "lex+fused-all"}
	if len(lg.Results) != len(wantVariants)*2 {
		t.Fatalf("%d results, want %d", len(lg.Results), len(wantVariants)*2)
	}
	lex := sum.LexicalGate
	if math.Abs(lg.Lexical.Recall-lex.Gold.Recall) > 1e-12 || math.Abs(lg.Lexical.FIR-lex.Negative.InjectRate) > 1e-12 || lg.Lexical.Gold != lex.Gold.Prompts || lg.Lexical.Negatives != lex.Negative.Prompts {
		t.Errorf("lexical reference through the pack path %+v differs from the hook gate %+v / %+v", lg.Lexical, lex.Gold, lex.Negative)
	}
	if lg.Lexical.Hit != lex.Gold.Hit || math.Abs(lg.Lexical.NegChars-lex.Negative.MeanChars) > 1e-9 || math.Abs(lg.Lexical.GoldChars-lex.Gold.MeanChars) > 1e-9 {
		t.Errorf("lexical hit/chars %+v differ from the hook gate", lg.Lexical)
	}
	col, rows := readTSV(t, filepath.Join(out, "logs-gate-heldout.tsv"))
	perPrompt := len(o.p.qs) + len(o.p.negs)
	if len(rows) != perPrompt*(1+len(wantVariants)*2) {
		t.Fatalf("heldout rows %d, want %d", len(rows), perPrompt*(1+len(wantVariants)*2))
	}
	type agg struct {
		rec, chars  [2]float64
		hit         float64
		gold, neg   int
		inj         int
		records, lg int
	}
	by := map[string]*agg{}
	for _, r := range rows {
		id := r[col["variant"]] + "@" + r[col["budget"]]
		a := by[id]
		if a == nil {
			a = &agg{}
			by[id] = a
		}
		gn, _ := strconv.Atoi(r[col["gold_n"]])
		gi, _ := strconv.Atoi(r[col["gold_injected"]])
		n, _ := strconv.Atoi(r[col["injected"]])
		nl, _ := strconv.Atoi(r[col["injected_log"]])
		ch, _ := strconv.ParseFloat(r[col["chars"]], 64)
		a.records += n
		a.lg += nl
		if r[col["set"]] == SetGold {
			a.gold++
			a.rec[0] += float64(gi) / float64(gn)
			a.chars[0] += ch
			if gi > 0 {
				a.hit++
			}
		} else {
			a.neg++
			a.chars[1] += ch
			if n > 0 {
				a.inj++
			}
		}
	}
	for _, res := range append(slices.Clone(lg.Results), lg.Lexical) {
		id := res.Variant + "@" + strconv.FormatFloat(res.Budget, 'g', -1, 64)
		if res.Variant == "lexical" {
			id = "lexical@-"
		}
		a := by[id]
		if a == nil {
			t.Errorf("no rows for %s", id)
			continue
		}
		if a.gold != res.Gold || a.neg != res.Negatives {
			t.Errorf("%s: rows %d/%d, summary %d/%d", id, a.gold, a.neg, res.Gold, res.Negatives)
			continue
		}
		if math.Abs(a.rec[0]/float64(a.gold)-res.Recall) > 1e-9 || math.Abs(a.hit/float64(a.gold)-res.Hit) > 1e-9 || math.Abs(float64(a.inj)/float64(a.neg)-res.FIR) > 1e-9 ||
			math.Abs(a.chars[0]/float64(a.gold)-res.GoldChars) > 1e-9 || math.Abs(a.chars[1]/float64(a.neg)-res.NegChars) > 1e-9 || a.records != res.Records || a.lg != res.LogRecords {
			t.Errorf("%s: TSV recomputation %+v disagrees with summary %+v", id, *a, res)
		}
		if len(res.Folds) != k && res.Variant != "lexical" {
			t.Errorf("%s: %d folds", id, len(res.Folds))
		}
	}
	for _, res := range lg.Results {
		for _, f := range res.Folds {
			if f.Feasible && f.TrainFIR > res.Budget+1e-9 {
				t.Errorf("%s budget %v fold %d: train FIR %v above the budget", res.Variant, res.Budget, f.Fold, f.TrainFIR)
			}
			for _, tight := range lg.Results {
				if tight.Variant == res.Variant && tight.Budget < res.Budget && tight.Folds[f.Fold].TrainRecall > f.TrainRecall+1e-9 {
					t.Errorf("%s fold %d: train recall %v at budget %v above %v at the looser budget %v", res.Variant, f.Fold, tight.Folds[f.Fold].TrainRecall, tight.Budget, f.TrainRecall, res.Budget)
				}
			}
		}
		if res.Variant == "reg" || res.Variant == "fused-reg" {
			if res.LogRecords != 0 {
				t.Errorf("%s injected log chunks", res.Variant)
			}
		}
	}
	data, _ := os.ReadFile(filepath.Join(out, "logs-gate-picks.tsv"))
	if got := strings.Count(strings.TrimSpace(string(data)), "\n"); got != len(wantVariants)*2*k {
		t.Errorf("pick rows %d, want %d", got, len(wantVariants)*2*k)
	}
	if !strings.Contains(fx.stdout.String(), "logs gate") {
		t.Errorf("stdout lacks the logs gate section:\n%s", fx.stdout.String())
	}
}

func randomLists(r *rand.Rand, goldSlug string, log bool) []vh {
	var out []vh
	for i := range r.IntN(5) {
		slug := fmt.Sprint("f", r.IntN(30))
		if i == 0 && goldSlug != "" && r.IntN(2) == 0 {
			slug = goldSlug
		}
		out = append(out, vh{key: slug + "#1", slug: slug, cos: r.Float64(), log: log})
	}
	slices.SortFunc(out, func(a, b vh) int {
		if a.cos > b.cos {
			return -1
		}
		return 1
	})
	return out
}

func TestCrossFitMatchesNaiveSearch(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	const k = 4
	var gold, neg []promptInput
	for range 70 {
		g := fmt.Sprint("f", r.IntN(30))
		gold = append(gold, promptInput{fold: r.IntN(k), goldLen: 1, gold: []string{g}, reg: randomLists(r, g, false), logs: randomLists(r, g, true)})
	}
	for range 150 {
		neg = append(neg, promptInput{fold: r.IntN(k), reg: randomLists(r, "", false), logs: randomLists(r, "", true)})
	}
	var regG, logG []KindGate
	regG = append(regG, KindGate{MinMargin: noMargin})
	logG = append(logG, KindGate{MinMargin: noMargin})
	for _, take := range []int{1, 2} {
		for _, c := range []float64{0.9, 0.7, 0.5, 0.3} {
			for _, m := range []float64{0.1, noMargin} {
				regG = append(regG, KindGate{Take: take, MinCos: c, MinMargin: m})
				logG = append(logG, KindGate{Take: take, MinCos: c, MinMargin: m})
			}
		}
	}
	l := newLayout(k, gold, neg)
	reg := make([]settingStats, len(regG))
	logs := make([]settingStats, len(logG))
	for i, g := range regG {
		reg[i] = l.stats(func(p *promptInput) []string { return slugsOf(g.apply(p.reg)) })
	}
	for i, g := range logG {
		logs[i] = l.stats(func(p *promptInput) []string { return slugsOf(g.apply(p.logs)) })
	}
	budgets := []float64{0.05, 0.2, 0.4}
	got := l.crossFit(reg, logs, budgets, 3)
	for b, budget := range budgets {
		for held := range k {
			bestIdx, bestRec, bestHit, bestNeg := -1, -1.0, -1, 0
			for i, rg := range regG {
				for j, lg := range logG {
					rec, hit, inj, goldN, negN := 0.0, 0, 0, 0, 0
					for _, p := range gold {
						if p.fold == held {
							continue
						}
						goldN++
						n := 0
						for _, h := range append(rg.apply(p.reg), lg.apply(p.logs)...) {
							if h.slug == p.gold[0] {
								n++
							}
						}
						if n > 0 {
							hit++
							rec++
						}
					}
					for _, p := range neg {
						if p.fold == held {
							continue
						}
						negN++
						if len(rg.apply(p.reg))+len(lg.apply(p.logs)) > 0 {
							inj++
						}
					}
					if float64(inj) > budget*float64(negN)+1e-9 {
						continue
					}
					better := bestIdx < 0 || math.Round(rec*1e9) > math.Round(bestRec*1e9) ||
						math.Round(rec*1e9) == math.Round(bestRec*1e9) && (hit > bestHit || hit == bestHit && inj < bestNeg)
					if better {
						bestIdx, bestRec, bestHit, bestNeg = i*len(logG)+j, rec, hit, inj
					}
				}
			}
			pk := got[b][held]
			if !pk.Feasible || pk.Reg*len(logG)+pk.Log != bestIdx {
				t.Errorf("budget %v held-out fold %d: crossFit picked %d/%d (feasible %v), naive search %d/%d", budget, held, pk.Reg, pk.Log, pk.Feasible, bestIdx/len(logG), bestIdx%len(logG))
			}
			if math.Abs(pk.Train.rec-bestRec) > 1e-9 || pk.Train.hit != bestHit || pk.Train.neg != bestNeg {
				t.Errorf("budget %v fold %d: train totals %+v, naive rec %v hit %d neg %d", budget, held, pk.Train, bestRec, bestHit, bestNeg)
			}
		}
	}
}

type poolFixture struct {
	ev       *vecEval
	qs       []*query
	queries  *vec.Set
	docs     *vec.Set
	byKey    map[string]store.Record
	keySlug  map[string]string
	sentence func(n int) string
}

func newPoolFixture(t *testing.T) *poolFixture {
	t.Helper()
	r := rand.New(rand.NewPCG(9, 13))
	vocab := make([]string, 25)
	for i := range vocab {
		vocab[i] = fmt.Sprintf("term%c%c%c", 'a'+i%26, 'a'+(i*5)%26, 'a'+(i*11)%26)
	}
	sentence := func(n int) string {
		w := make([]string, n)
		for i := range w {
			w[i] = vocab[r.IntN(len(vocab))]
		}
		return strings.Join(w, " ")
	}
	ctx := t.Context()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "p.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	put := func(key string, kind store.Kind) {
		title, body := sentence(3), sentence(18)
		if _, err := s.Put(ctx, store.PutRequest{Key: key, Kind: kind, Scopes: []string{store.GlobalScope}, Title: &title, Body: &body, Gate: store.Gate{Dup: 2, Rel: 2, MinShared: 1000}}); err != nil {
			t.Fatal(err)
		}
	}
	keySlug := map[string]string{}
	for i := range 40 {
		key := fmt.Sprintf("reg_%03d", i)
		put(key, store.Fact)
		keySlug[key] = key
	}
	for f := range 18 {
		for c := 1; c <= 4; c++ {
			key := fmt.Sprintf("file%02d#%d", f, c)
			put(key, store.Log)
			keySlug[key] = fmt.Sprintf("file%02d", f)
		}
	}
	recs, err := s.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	const dim = 10
	docs := &vec.Set{Dim: dim}
	byKey := map[string]store.Record{}
	base := map[string][]float32{}
	for _, rc := range recs {
		docs.Names = append(docs.Names, rc.Key)
		byKey[rc.Key] = rc
		row := randomUnit(r, dim)
		if b, ok := base[keySlug[rc.Key]]; ok && rc.Kind == store.Log {
			row = slices.Clone(b)
			for i := range row {
				row[i] += float32(r.NormFloat64() * 0.05)
			}
			var n float64
			for _, x := range row {
				n += float64(x) * float64(x)
			}
			for i := range row {
				row[i] /= float32(math.Sqrt(n))
			}
		}
		if rc.Kind == store.Log {
			if _, ok := base[keySlug[rc.Key]]; !ok {
				base[keySlug[rc.Key]] = slices.Clone(row)
			}
		}
		docs.Data = append(docs.Data, row...)
	}
	queries := &vec.Set{Dim: dim}
	var qs []*query
	for i := range 30 {
		q := &query{Prompt: Prompt{Session: "s", Turn: i + 1, Text: sentence(6), Query: sentence(2 + i%5)}}
		qs = append(qs, q)
		queries.Names = append(queries.Names, queryID(q))
		queries.Data = append(queries.Data, randomUnit(r, dim)...)
	}
	cfg := Config{VecWeight: 1, VecDepth: 60, LogsGate: true, LexicalRef: true, Pools: AllPools, Recall: recall.DefaultSettings()}
	ev := &vecEval{s: s, cfg: cfg, docs: docs, queries: queries, qIndex: map[string]int{}, index: map[string]int{}, keySlug: keySlug, isLog: make([]bool, len(docs.Names)), pool: 12, settings: cfg.Recall}
	for i, n := range queries.Names {
		ev.qIndex[n] = i
	}
	for i, n := range docs.Names {
		ev.index[n] = i
		ev.isLog[i] = byKey[n].Kind == store.Log
		ev.eligAll = append(ev.eligAll, i)
		if byKey[n].Kind != store.Log {
			ev.elig = append(ev.elig, i)
		}
	}
	return &poolFixture{ev: ev, qs: qs, queries: queries, docs: docs, byKey: byKey, keySlug: keySlug, sentence: sentence}
}

func TestPoolListsAgainstNaive(t *testing.T) {
	pf := newPoolFixture(t)
	ctx := t.Context()
	ev, qs, queries, docs, byKey, keySlug, s := pf.ev, pf.qs, pf.queries, pf.docs, pf.byKey, pf.keySlug, pf.ev.s
	repeats, logTop := 0, 0
	for _, q := range qs {
		got, err := ev.one(ctx, q, false)
		if err != nil {
			t.Fatal(err)
		}
		qv := queries.Row(ev.qIndex[queryID(q)])
		type cs struct {
			key string
			cos float64
			log bool
		}
		var all []cs
		for i, n := range docs.Names {
			all = append(all, cs{n, naiveCos(qv, docs.Row(i)), byKey[n].Kind == store.Log})
		}
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].cos != all[j].cos {
				return all[i].cos > all[j].cos
			}
			return all[i].key < all[j].key
		})
		naivePool := func(src []cs, n int) []string {
			seen := map[string]bool{}
			var out []string
			for _, x := range src {
				if seen[keySlug[x.key]] {
					continue
				}
				seen[keySlug[x.key]] = true
				out = append(out, x.key)
				if len(out) == n {
					break
				}
			}
			return out
		}
		var regOnly []cs
		for _, x := range all {
			if !x.log {
				regOnly = append(regOnly, x)
			}
		}
		naiveKinds := func(src []cs, n int) []string {
			seen := map[string]bool{}
			var out []string
			perKind := map[bool]int{}
			for _, x := range src {
				if seen[keySlug[x.key]] || perKind[x.log] == n {
					continue
				}
				seen[keySlug[x.key]] = true
				perKind[x.log]++
				out = append(out, x.key)
			}
			return out
		}
		keysOfPool := func(name string) []string { return keysOf(got.pools[name]) }
		if want := naivePool(regOnly, 12); !slices.Equal(keysOfPool(PoolReg), want) {
			t.Fatalf("reg pool %v want %v", keysOfPool(PoolReg), want)
		}
		wantAll := naiveKinds(all, 12)
		if !slices.Equal(keysOfPool(PoolAll), wantAll) {
			t.Fatalf("all pool %v want %v", keysOfPool(PoolAll), wantAll)
		}
		allReg, allLogs := splitKinds(got.pools[PoolAll])
		if want := naivePool(regOnly, 12); !slices.Equal(keysOf(allReg), want) {
			t.Fatalf("regular side of the all pool %v, the regular-only ranking gives %v", keysOf(allReg), want)
		}
		var logOnly []cs
		for _, x := range all {
			if x.log {
				logOnly = append(logOnly, x)
			}
		}
		if want := naivePool(logOnly, 12); !slices.Equal(keysOf(allLogs), want) {
			t.Fatalf("log side of the all pool %v, the log-only ranking gives %v", keysOf(allLogs), want)
		}
		var rawWant []string
		for _, x := range all[:12] {
			rawWant = append(rawWant, x.key)
		}
		if !slices.Equal(keysOf(got.raw), rawWant) {
			t.Fatalf("raw top-12 %v want %v", keysOf(got.raw), rawWant)
		}
		seenFile := map[string]bool{}
		for _, h := range got.raw {
			if h.log && seenFile[h.slug] {
				repeats++
			}
			seenFile[h.slug] = true
		}
		if got.raw[0].log {
			logTop++
		}
		hits, err := s.Search(ctx, store.Query{Text: q.Query, Limit: 1000, Depth: max(recall.SearchDepth, 12), ExcludeKinds: []store.Kind{store.Log}, Mode: store.Fused})
		if err != nil {
			t.Fatal(err)
		}
		type fs struct {
			key   string
			score float64
			bm    float64
		}
		scores := map[string]*fs{}
		for _, h := range hits {
			scores[h.Record.Key] = &fs{h.Record.Key, h.Score, h.WordsBM25 + h.TriBM25}
		}
		for rank, k := range naivePool(all, 60) {
			f := scores[k]
			if f == nil {
				f = &fs{key: k}
				scores[k] = f
			}
			f.score += 1 / float64(store.RRFK+rank+1)
		}
		var ref []*fs
		for _, f := range scores {
			ref = append(ref, f)
		}
		sort.Slice(ref, func(i, j int) bool {
			a, b := ref[i], ref[j]
			if a.score != b.score {
				return a.score > b.score
			}
			if a.bm != b.bm {
				return a.bm > b.bm
			}
			return a.key < b.key
		})
		var fusedRef []cs
		for _, f := range ref {
			fusedRef = append(fusedRef, cs{key: f.key, log: byKey[f.key].Kind == store.Log})
		}
		if want := naiveKinds(fusedRef, 12); !slices.Equal(keysOfPool(PoolFusedAll), want) {
			t.Fatalf("fused-all pool %v want %v", keysOfPool(PoolFusedAll), want)
		}
		for name, list := range got.pools {
			files := map[string]bool{}
			for _, h := range list {
				if files[h.slug] {
					t.Fatalf("%s pool holds two chunks of %s", name, h.slug)
				}
				files[h.slug] = true
				if h.log != (byKey[h.key].Kind == store.Log) {
					t.Fatalf("%s: %s log flag %v", name, h.key, h.log)
				}
				if (name == PoolReg || name == PoolFusedReg) && h.log {
					t.Fatalf("%s pool holds log chunk %s", name, h.key)
				}
			}
			reg, logs := splitKinds(list)
			if len(reg) > 12 || len(logs) > 12 {
				t.Fatalf("%s pool of %d regular and %d log files", name, len(reg), len(logs))
			}
		}
	}
	if repeats == 0 {
		t.Error("fixture never repeats a file in the raw top-12, dedup is untested")
	}
	if logTop == 0 {
		t.Error("fixture never leads with a log chunk")
	}
}

func plantedLayout(fold2Gold, fold2Neg float64, fold2Negs int) (*layout, []settingStats, []settingStats, []KindGate) {
	var gold, neg []promptInput
	for f := range 2 {
		for range 6 {
			gold = append(gold, promptInput{fold: f, goldLen: 1, gold: []string{"A"}, reg: []vh{{key: "A", slug: "A", cos: 0.8}}})
		}
		for i := range 10 {
			p := promptInput{fold: f}
			if i < 5 {
				p.reg = []vh{{key: "x", slug: "x", cos: 0.5}}
			}
			neg = append(neg, p)
		}
	}
	for range 6 {
		gold = append(gold, promptInput{fold: 2, goldLen: 1, gold: []string{"A"}, reg: []vh{{key: "A", slug: "A", cos: fold2Gold}}})
	}
	for i := range fold2Negs {
		p := promptInput{fold: 2}
		if fold2Neg > 0 && i%2 == 0 {
			p.reg = []vh{{key: "x", slug: "x", cos: fold2Neg}}
		}
		neg = append(neg, p)
	}
	grid := []KindGate{{MinMargin: noMargin}}
	for _, c := range []float64{0.3, 0.6, 0.9} {
		grid = append(grid, KindGate{Take: 1, MinCos: c, MinMargin: noMargin})
	}
	l := newLayout(3, gold, neg)
	reg := make([]settingStats, len(grid))
	for i, g := range grid {
		reg[i] = l.stats(func(p *promptInput) []string { return slugsOf(g.apply(p.reg)) })
	}
	return l, reg, []settingStats{l.stats(offItems)}, grid
}

func TestCrossFitNeverSeesHeldOutFold(t *testing.T) {
	budgets := []float64{0.25}
	l, reg, logs, grid := plantedLayout(0.35, 0, 40)
	picks := l.crossFit(reg, logs, budgets, 2)
	got := grid[picks[0][2].Reg]
	if got.MinCos != 0.6 || !picks[0][2].Feasible {
		t.Errorf("fold 2 holds a signal only it can see (gold at cos 0.35, 40 quiet negatives, which would make cos 0.3 the pick); its pick must come from folds 0 and 1 alone, got %+v feasible %v", got, picks[0][2].Feasible)
	}
	if p := picks[0][2].Train; p.goldN != 12 || p.nN != 20 {
		t.Errorf("fold 2 trained on %d gold and %d negatives, want 12 and 20", p.goldN, p.nN)
	}
	if g := grid[picks[0][0].Reg]; g.MinCos != 0.3 {
		t.Errorf("fold 0 trains on folds 1 and 2 and should pick cos 0.3 there, got %+v", g)
	}
	variants := []struct {
		gold, neg float64
		negs      int
	}{{0.95, 0.99, 8}, {0.1, 0, 3}, {0.7, 0.2, 60}}
	for _, v := range variants {
		l2, reg2, logs2, _ := plantedLayout(v.gold, v.neg, v.negs)
		other := l2.crossFit(reg2, logs2, budgets, 1)
		if other[0][2].Reg != picks[0][2].Reg || other[0][2].Log != picks[0][2].Log || other[0][2].Train != picks[0][2].Train {
			t.Errorf("rewriting fold 2 changed fold 2's own pick: %+v vs %+v", other[0][2], picks[0][2])
		}
	}
}

func TestLogsGatePicksIgnoreTheHeldOutFoldEndToEnd(t *testing.T) {
	pf := newPoolFixture(t)
	ev := pf.ev
	ev.cfg.Folds, ev.cfg.FIRBudgets = 3, []float64{0.5}
	sess := map[int]string{}
	for i := 0; len(sess) < 3; i++ {
		s := fmt.Sprint("sess", i)
		if _, ok := sess[foldK(s, 3)]; !ok {
			sess[foldK(s, 3)] = s
		}
	}
	var negRes []qres
	var negQ []*query
	run := func(plant bool) *LogsGateSummary {
		var gold, neg []*query
		var gres, nres []qres
		for i, q0 := range pf.qs {
			q := &query{Prompt: q0.Prompt}
			q.Session = sess[i%3]
			ev.qIndex[queryID(q)] = ev.qIndex[queryID(q0)]
			res, err := ev.one(t.Context(), q, true)
			if err != nil {
				t.Fatal(err)
			}
			if i >= 24 {
				neg, nres = append(neg, q), append(nres, res)
				continue
			}
			q.gold = []string{"nowhere"}
			if plant && i%3 == 2 {
				q.gold = []string{res.pools[PoolReg][0].slug}
			}
			gold, gres = append(gold, q), append(gres, res)
		}
		lg, _, err := ev.logsGate(pf.byKey, map[string]bool{}, gold, neg, gres, nres)
		if err != nil {
			t.Fatal(err)
		}
		negQ, negRes = neg, nres
		return lg
	}
	planted := run(true)
	for _, a := range planted.Results {
		for _, f := range a.Folds {
			if f.Lexical {
				continue
			}
			n, inj := 0, 0
			for i, q := range negQ {
				if foldK(q.Session, 3) != f.Fold {
					continue
				}
				n++
				reg, logs := splitKinds(negRes[i].pools[a.Pool])
				if len(f.Regular.apply(reg))+len(f.Log.apply(logs)) > 0 {
					inj++
				}
			}
			if n != f.Negatives || n > 0 && math.Abs(float64(inj)/float64(n)-f.FIR) > 1e-9 {
				t.Errorf("%s fold %d: held-out FIR %v over %d negatives, its own pick gives %d of %d", a.Variant, f.Fold, f.FIR, f.Negatives, inj, n)
			}
		}
	}
	plain := run(false)
	if len(planted.Results) != len(plain.Results) || len(planted.Results) == 0 {
		t.Fatalf("%d and %d results", len(planted.Results), len(plain.Results))
	}
	moved := false
	for i, a := range planted.Results {
		b := plain.Results[i]
		fa, fb := a.Folds[2], b.Folds[2]
		if fa.Regular != fb.Regular || fa.Lexical != fb.Lexical || fa.Log != fb.Log || fa.TrainRecall != fb.TrainRecall || fa.TrainFIR != fb.TrainFIR || fa.Feasible != fb.Feasible {
			t.Errorf("%s: fold 2's pick depends on fold 2's own gold: %+v vs %+v", a.Variant, fa, fb)
		}
		if !fa.Lexical && fa.Recall != 0 {
			t.Errorf("%s: fold 2 recall %v, its training folds hold no signal so nothing can be injected correctly", a.Variant, fa.Recall)
		}
		for _, f := range []int{0, 1} {
			if a.Folds[f].Regular != b.Folds[f].Regular || a.Folds[f].Log != b.Folds[f].Log {
				moved = true
			}
		}
	}
	if !moved {
		t.Error("planting the signal in fold 2 moved no other fold's pick, the test has no power")
	}
}

func TestLexicalVariantsReproduceTheShippedGateWithoutLogs(t *testing.T) {
	pf := newPoolFixture(t)
	ev := pf.ev
	ev.cfg.Folds, ev.cfg.FIRBudgets, ev.cfg.LexRegular = 3, []float64{0.2, 1}, true
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
		if _, logs := splitKinds(res.pools[PoolAll]); i%2 == 0 && len(logs) > 0 {
			q.gold = []string{logs[0].slug}
		}
		gold, gres = append(gold, q), append(gres, res)
	}
	lg, sets, err := ev.logsGate(pf.byKey, map[string]bool{}, gold, neg, gres, nres)
	if err != nil {
		t.Fatal(err)
	}
	type key struct {
		set     string
		session string
		turn    int
	}
	lex := map[key]heldRow{}
	for _, s := range sets {
		if s.variant == lexicalVariant {
			for _, r := range s.rows {
				lex[key{r.set, r.q.Session, r.q.Turn}] = r
			}
		}
	}
	if len(lex) != len(gold)+len(neg) {
		t.Fatalf("%d lexical rows", len(lex))
	}
	checked, withLog := 0, 0
	for _, s := range sets {
		if !strings.HasPrefix(s.variant, "lex+") {
			continue
		}
		for _, r := range s.rows {
			base := lex[key{r.set, r.q.Session, r.q.Turn}]
			if r.injected-r.injLog != base.injected || r.goldInj-r.goldInjLog != base.goldInj {
				t.Errorf("%s budget %s: the regular part %d/%d differs from the shipped gate %d/%d", s.variant, s.budget, r.injected-r.injLog, r.goldInj-r.goldInjLog, base.injected, base.goldInj)
			}
			if r.injLog == 0 && (r.injected != base.injected || r.goldInj != base.goldInj || r.chars != base.chars || r.dropped != base.dropped) {
				t.Errorf("%s budget %s: with no log injected the row %+v differs from the shipped gate %+v", s.variant, s.budget, r, base)
			}
			checked++
			if r.injLog > 0 {
				withLog++
			}
		}
	}
	if checked == 0 || withLog == 0 || withLog == checked {
		t.Errorf("fixture must mix prompts with and without a log chunk: %d of %d", withLog, checked)
	}
	if lg.Lexical.Gold != len(gold) {
		t.Errorf("lexical gold %d", lg.Lexical.Gold)
	}
}

func TestDecideRowCountsAGoldFileOnceAndByExactSlug(t *testing.T) {
	pf := newPoolFixture(t)
	ev := pf.ev
	chunks := []vh{{key: "file00#1", slug: "file00", cos: 0.9, log: true}, {key: "file00#2", slug: "file00", cos: 0.8, log: true}, {key: "file01#1", slug: "file01", cos: 0.7, log: true}}
	p := &promptInput{logs: chunks}
	gate := KindGate{Take: 3, MinCos: 0.1, MinMargin: noMargin}
	row := ev.decideRow(p, &query{Prompt: Prompt{Query: pf.sentence(3)}, gold: []string{"file00", "file0", "file1", "file"}}, SetGold, regChoice{gate: KindGate{MinMargin: noMargin}}, gate, pf.byKey)
	if row.injected != 3 || row.injLog != 3 {
		t.Fatalf("injected %d, log %d, want 3 and 3 (dropped %d)", row.injected, row.injLog, row.dropped)
	}
	if row.goldInj != 1 || row.goldInjLog != 1 {
		t.Errorf("two chunks of one gold file count %d (log %d), want 1; similar slugs file0, file1 and file must not match", row.goldInj, row.goldInjLog)
	}
	l := newLayout(1, []promptInput{{fold: 0, goldLen: 4, gold: []string{"file00", "file0", "file1", "file"}, logs: chunks}}, nil)
	st := l.stats(func(p *promptInput) []string { return slugsOf(gate.apply(p.logs)) })
	if st.rec[0] != 0.25 {
		t.Errorf("stats count %v of the gold files, want 0.25", st.rec[0])
	}
}

func TestDecideRowPackerBudgetWithChunks(t *testing.T) {
	r := rand.New(rand.NewPCG(21, 22))
	ev := &vecEval{cfg: Config{Recall: recall.DefaultSettings()}}
	body := func(n int) string {
		var b strings.Builder
		for b.Len() < n {
			fmt.Fprintf(&b, "word%d ", r.IntN(500))
		}
		return b.String()[:n]
	}
	sawDrop, sawLog := false, false
	for range 300 {
		ev.cfg.Recall.MaxChars = []int{900, 1500, recall.DefaultMaxChars}[r.IntN(3)]
		byKey := map[string]store.Record{}
		var regs, logs []vh
		for i := range 1 + r.IntN(4) {
			k := fmt.Sprint("reg", i)
			byKey[k] = store.Record{Key: k, Kind: store.Fact, Title: "t", Body: body(100 + r.IntN(1500))}
			regs = append(regs, vh{key: k, slug: k, cos: 0.9})
		}
		for i := range r.IntN(3) {
			k := fmt.Sprint("file", i, "#1")
			byKey[k] = store.Record{Key: k, Kind: store.Log, Title: "t", Body: body(900 + r.IntN(900))}
			logs = append(logs, vh{key: k, slug: fmt.Sprint("file", i), cos: 0.9, log: true})
		}
		p := &promptInput{reg: regs, logs: logs}
		row := ev.decideRow(p, &query{Prompt: Prompt{Query: "word1 word2"}}, SetNegative, regChoice{gate: KindGate{Take: 4, MinCos: 0.1, MinMargin: noMargin}}, KindGate{Take: 2, MinCos: 0.1, MinMargin: noMargin}, byKey)
		want := len(regs) + len(logs)
		if row.injected+row.dropped != want {
			t.Fatalf("injected %d + dropped %d, picked %d", row.injected, row.dropped, want)
		}
		if row.chars > ev.cfg.Recall.MaxChars {
			t.Fatalf("%d chars above the %d budget", row.chars, ev.cfg.Recall.MaxChars)
		}
		if row.dropped > 0 {
			sawDrop = true
			if row.injLog > 0 && row.injected-row.injLog < len(regs) {
				t.Fatalf("a regular record was dropped while a log chunk stayed: %+v", row)
			}
		}
		if row.injLog > 0 {
			sawLog = true
		}
	}
	if !sawDrop || !sawLog {
		t.Errorf("fixture must exercise both a drop and a kept chunk: drop %v log %v", sawDrop, sawLog)
	}
}

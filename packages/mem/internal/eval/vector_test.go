package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"mem/internal/recall"
	"mem/internal/store"
	"mem/internal/vec"
)

func TestFuse(t *testing.T) {
	words := []store.ListHit{{Key: "a", Rank: 1, Score: 9}, {Key: "b", Rank: 2, Score: 8}, {Key: "c", Rank: 3, Score: 7}}
	tri := []store.ListHit{{Key: "c", Rank: 1, Score: 5}, {Key: "a", Rank: 2, Score: 4}}
	got := fuse(words, tri, []string{"d", "c"}, 1, 10)
	score := map[string]float64{
		"a": 1.0/61 + store.TriWeight/62,
		"b": 1.0 / 62,
		"c": 1.0/63 + store.TriWeight/61 + 1.0/62,
		"d": 1.0 / 61,
	}
	var keys []string
	for _, g := range got {
		keys = append(keys, g.key)
		if math.Abs(g.score-score[g.key]) > 1e-12 {
			t.Errorf("%s: score %v want %v", g.key, g.score, score[g.key])
		}
	}
	if !slices.Equal(keys, []string{"c", "a", "d", "b"}) {
		t.Errorf("order %v", keys)
	}
	if lim := fuse(words, tri, []string{"d", "c"}, 1, 2); len(lim) != 2 || lim[0].key != "c" {
		t.Errorf("limit: %v", lim)
	}
	lex := fuse(words, tri, []string{"d", "c"}, 0, 10)
	var lk []string
	for _, g := range lex {
		lk = append(lk, g.key)
	}
	if !slices.Equal(lk, []string{"a", "c", "b"}) {
		t.Errorf("weight 0 must reproduce the lexical order and drop vector-only keys: %v", lk)
	}
	tied := fuse([]store.ListHit{{Key: "b", Rank: 1, Score: 3}}, []store.ListHit{{Key: "a", Rank: 1, Score: 3}}, nil, 1, 10)
	if len(tied) != 2 || tied[0].key != "b" {
		t.Errorf("words beat trigram at rank 1: %v", tied)
	}
}

func TestGateInject(t *testing.T) {
	list := []vh{{key: "a", cos: 0.8}, {key: "b", cos: 0.7}, {key: "c", cos: 0.4}}
	for _, c := range []struct {
		take int
		t, m float64
		want []string
	}{
		{1, 0.5, noMargin, []string{"a"}},
		{4, 0.5, noMargin, []string{"a", "b"}},
		{4, 0.3, noMargin, []string{"a", "b", "c"}},
		{2, 0.3, noMargin, []string{"a", "b"}},
		{1, 0.85, noMargin, nil},
		{1, 0.5, 0.05, []string{"a"}},
		{1, 0.5, 0.2, nil},
		{1, 0.5, 0.1, []string{"a"}},
	} {
		got := gateInject(list, c.take, c.t, c.m)
		if !slices.Equal(got, c.want) {
			t.Errorf("take %d T %.2f M %.2f: %v want %v", c.take, c.t, c.m, got, c.want)
		}
	}
	if got := gateInject([]vh{{key: "a", cos: 0.2}, {key: "b", cos: 0.6}}, 1, 0.1, 0); got != nil {
		t.Errorf("negative margin passes: %v", got)
	}
	if got := gateInject(nil, 1, 0, noMargin); got != nil {
		t.Errorf("empty list: %v", got)
	}
	if got := gateInject([]vh{{key: "a", cos: 0.9}}, 1, 0.5, 0.3); !slices.Equal(got, []string{"a"}) {
		t.Errorf("single candidate: %v", got)
	}
}

func TestWorstFold(t *testing.T) {
	rows := []VecRow{
		{Folds: [2]Fold{{Recall: 0.4, FalseRate: 0.2}, {Recall: 0.1, FalseRate: 0.1}}},
		{Folds: [2]Fold{{Recall: 0.3, FalseRate: 0.05}, {Recall: 0.3, FalseRate: 0.1}}},
		{Folds: [2]Fold{{Recall: 0.1, FalseRate: 0.0}, {Recall: 0.1, FalseRate: 0.0}}},
	}
	best := markWorstFold(rows)
	if best != 2 || !rows[2].WorstBest || rows[0].WorstBest || rows[1].WorstBest {
		t.Fatalf("best %d %+v", best, rows)
	}
	for i, want := range []float64{-2.0 / 3, 0, 0.25} {
		if math.Abs(rows[i].Worst-want) > 1e-9 {
			t.Errorf("row %d worst-fold utility %v, want %v", i, rows[i].Worst, want)
		}
	}
	for _, r := range rows {
		if r.Worst > math.Min(r.Utility[0], r.Utility[1])+1e-12 {
			t.Errorf("worst %v above the weaker fold %v", r.Worst, r.Utility)
		}
	}
}

type oracle struct {
	dir, export string
	gold        map[string]string
	docKeys     []string
	wantHit     int
	p           *prepared
}

func buildOracle(t *testing.T, fx *evalFixture, dim int) *oracle {
	t.Helper()
	writeFiles(t, fx.root, map[string]string{
		"cases.json": `[{"name": "cooler", "prompt": "What does the 0xDF command do to the BS3 Pro cooler?", "want": ["reference_bs3pro"]},
			{"name": "unrelated", "prompt": "write a haiku about autumn leaves falling in the park", "none": true}]`,
	})
	o := &oracle{export: filepath.Join(fx.root, "export"), dir: filepath.Join(fx.root, "vecs"), gold: map[string]string{}}
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--export-texts", o.export, "--out", filepath.Join(fx.root, "work")), fx.e); code != 0 {
		t.Fatalf("export: %d %s", code, fx.stderr.String())
	}
	cfg := Config{Glob: filepath.Join(fx.projects, "*.jsonl"), LegacyDir: fx.mem, Project: "repo", Out: filepath.Join(fx.root, "prep"),
		Limit: DefaultLimit, Depth: DefaultDepth, Tail: DefaultTail, MinChars: DefaultMinChars, MaxQuery: DefaultMaxQuery, Seed: DefaultSeed,
		Cases: filepath.Join(fx.root, "cases.json"), Recall: recall.DefaultSettings()}
	p, err := prepare(t.Context(), fx.e, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.s.Close() })
	o.p = p
	docs := readJSONL[DocText](t, filepath.Join(o.export, DocsFile))
	slugKey := map[string]string{}
	kind := map[string]store.Kind{}
	recs, err := p.s.List(t.Context(), store.Filter{AllStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		kind[r.Key] = r.Kind
		if s := p.keySlug[r.Key]; s != "" {
			if _, ok := slugKey[s]; !ok || r.Kind != store.Log {
				slugKey[s] = r.Key
			}
		}
	}
	axis := map[string]int{}
	var dkeys []string
	var ddata []float32
	for i, d := range docs {
		axis[d.Key] = i
		dkeys = append(dkeys, d.Key)
		row := make([]float32, dim)
		row[i] = 1
		ddata = append(ddata, row...)
	}
	o.docKeys = dkeys
	qs := readJSONL[QueryText](t, filepath.Join(o.export, QueriesFile))
	var qids []string
	var qdata []float32
	byID := map[string]*query{}
	for _, q := range append(append(slices.Clone(p.qs), p.negs...), p.cases...) {
		byID[p.id(q)] = q
	}
	for _, q := range qs {
		row := make([]float32, dim)
		row[dim-1], row[axis[dkeys[0]]] = 0.8, 0.6
		switch {
		case q.Set == SetGold:
			var pick string
			for _, g := range byID[q.ID].gold {
				k := slugKey[g]
				if pick == "" || kind[pick] == store.Log && kind[k] != store.Log {
					pick = k
				}
			}
			row[dim-1], row[axis[dkeys[0]]], row[axis[pick]] = 0, 0, 1
			o.gold[q.ID] = pick
			if kind[pick] != store.Log {
				o.wantHit++
			}
		case q.ID == "case:cooler":
			row[dim-1], row[axis[dkeys[0]]], row[axis["reference_bs3pro"]] = 0, 0, 1
		}
		qids = append(qids, q.ID)
		qdata = append(qdata, row...)
	}
	if err := vec.WriteSet(o.dir, vec.PartDocs, dkeys, ddata, dim); err != nil {
		t.Fatal(err)
	}
	if err := vec.WriteSet(o.dir, vec.PartQueries, qids, qdata, dim); err != nil {
		t.Fatal(err)
	}
	if err := vec.WriteMeta(o.dir, &vec.Meta{Model: "oracle", Dim: dim}); err != nil {
		t.Fatal(err)
	}
	return o
}

func loadVectorSummary(t *testing.T, out string) VectorSummary {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "vector-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sum VectorSummary
	if err := json.Unmarshal(data, &sum); err != nil {
		t.Fatal(err)
	}
	return sum
}

func metricOf(t *testing.T, sum VectorSummary, source, gold string) VecMetric {
	t.Helper()
	for _, m := range sum.Metrics {
		if m.Source == source && m.Gold == gold {
			return m
		}
	}
	t.Fatalf("no metric %s/%s in %+v", source, gold, sum.Metrics)
	return VecMetric{}
}

func TestEvaluateVectors(t *testing.T) {
	fx := newEvalFixture(t)
	const dim = 16
	o := buildOracle(t, fx, dim)
	if o.wantHit == 0 || o.wantHit == len(o.p.qs) {
		t.Fatalf("fixture must hold gold prompts with and without a non-log record: %d of %d", o.wantHit, len(o.p.qs))
	}
	out := filepath.Join(fx.root, "vout")
	args := fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out)
	if code := Run(args, fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
	stdout := fx.stdout.String()
	for _, leak := range []string{longMarker, "git push the agent-guard", "zfs readahead batch patches", "Never send 0xDF"} {
		if strings.Contains(stdout, leak) {
			t.Errorf("stdout leaks text %q", leak)
		}
	}
	for _, name := range []string{"vector-summary.json", "vector-summary.tsv", "vector-sweep.tsv", "vector-queries.tsv"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", name, info, err)
			continue
		}
		data, _ := os.ReadFile(filepath.Join(out, name))
		for _, leak := range []string{longMarker, "zfs readahead batch patches"} {
			if strings.Contains(string(data), leak) {
				t.Errorf("%s leaks prompt text", name)
			}
		}
	}
	sum := loadVectorSummary(t, out)
	n := len(o.p.qs)
	if sum.Gold != n || sum.Negatives != 2 || sum.CaseCount != 2 || sum.Dim != dim || sum.Model != "oracle" || sum.FusionMismatch != 0 || sum.Eligible == 0 || sum.Eligible >= sum.Records {
		t.Fatalf("summary header %+v", sum)
	}
	if err := hookSearch(t.Context(), o.p.s, o.p.qs, recall.DefaultSettings(), 2); err != nil {
		t.Fatal(err)
	}
	lexHits := 0
	for _, q := range o.p.qs {
		for _, c := range q.cands {
			if slices.Contains(q.gold, o.p.keySlug[c.Hit.Record.Key]) {
				lexHits++
				break
			}
		}
	}
	lex := metricOf(t, sum, "lexical", "read")
	if lex.Prompts != n || lex.Cand12Hits != lexHits {
		t.Errorf("lexical candidate recall %d of %d, hook candidates hold %d", lex.Cand12Hits, lex.Prompts, lexHits)
	}
	v := metricOf(t, sum, "vector", "read")
	if v.Cand12Hits != o.wantHit || v.Prompts != n || math.Abs(v.MRR10-float64(o.wantHit)/float64(n)) > 1e-9 || v.Hit4 < v.Cand12-1e-9 {
		t.Errorf("vector %+v, want %d of %d at rank 1", v, o.wantHit, n)
	}
	if all := metricOf(t, sum, "vector-all", "read"); all.Cand12Hits != n || all.MRR10 != 1 {
		t.Errorf("vector-all %+v", all)
	}
	if f := metricOf(t, sum, "fused", "read"); f.Cand12Hits < lex.Cand12Hits || f.Cand12Hits < v.Cand12Hits {
		t.Errorf("fused %+v loses to lexical %d or vector %d", f, lex.Cand12Hits, v.Cand12Hits)
	}
	if got, want := sum.LexicalGate.Gold, hookStats(SetGold, o.p.qs, outcomes(o.p.qs), o.p.keySlug); got.Injecting != want.Injecting || got.Recall != want.Recall || got.Records != want.Records {
		t.Errorf("lexical gate reference %+v, hook %+v", got, want)
	}
	if len(sum.Groups) != 4 {
		t.Fatalf("groups %d", len(sum.Groups))
	}
	found := false
	data, _ := os.ReadFile(filepath.Join(out, "vector-sweep.tsv"))
	rows := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(rows) != 1+len(VecGrid()) {
		t.Errorf("sweep rows %d, grid %d", len(rows)-1, len(VecGrid()))
	}
	for _, g := range sum.Groups {
		if g.Source == "vector" && g.Take == 1 {
			found = true
			if g.WorstBest == nil || g.Knee == nil {
				t.Errorf("group %+v", g)
			}
		}
	}
	if !found {
		t.Error("no vector take-1 group")
	}
	row, ok := sum.Row("vector", 1, 0.7, noMargin)
	if !ok || row.Negative.Injecting != 0 || math.Abs(row.Gold.Hit-float64(o.wantHit)/float64(n)) > 1e-9 {
		t.Errorf("vector T 0.7 row %+v %v", row, ok)
	}
	if m, ok := sum.Row("vector", 1, 0.7, 0.12); !ok || m.Gold.Hit != row.Gold.Hit || m.Negative.Injecting != 0 {
		t.Errorf("margin row %+v", m)
	}
	if m, ok := sum.Row("vector", 1, 0.5, noMargin); !ok || m.Negative.Injecting != 2 || m.Negative.InjectRate != 1 {
		t.Errorf("vector T 0.5 row lets the 0.6 negatives through: %+v", m)
	}
	if len(sum.Cases) != 2 {
		t.Fatalf("cases %+v", sum.Cases)
	}
	for _, c := range sum.Cases {
		if c.Name == "cooler" && (len(c.Vector) == 0 || c.Vector[0].Key != "reference_bs3pro" || c.Vector[0].Cos < 0.99) {
			t.Errorf("cooler case %+v", c)
		}
	}
}

func TestEvaluateVectorsWeightZeroIsLexical(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	out := filepath.Join(fx.root, "vout")
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out, "--vec-weight", "0"), fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
	sum := loadVectorSummary(t, out)
	for _, gold := range []string{"read", "read-not-written"} {
		lex, fused := metricOf(t, sum, "lexical", gold), metricOf(t, sum, "fused", gold)
		fused.Source = lex.Source
		if lex != fused {
			t.Errorf("%s: fused with weight 0 %+v differs from lexical %+v", gold, fused, lex)
		}
	}
}

func TestEvaluateVectorsDocsFromAnotherRun(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	docsDir := filepath.Join(fx.root, "docs-run")
	for _, name := range []string{"docs.f32", "docs.keys", "meta.json"} {
		data, err := os.ReadFile(filepath.Join(o.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(docsDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(docsDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"docs.f32", "docs.keys"} {
		if err := os.Remove(filepath.Join(o.dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(fx.root, "vout")
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--docs-vectors", docsDir, "--out", out), fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
	sum := loadVectorSummary(t, out)
	if v := metricOf(t, sum, "vector", "read"); v.Cand12Hits != o.wantHit || sum.DocsVectors != docsDir {
		t.Errorf("vector %+v docs from %q", v, sum.DocsVectors)
	}
	meta, _ := vec.ReadMeta(docsDir)
	meta.Dim = 8
	if err := vec.WriteMeta(docsDir, meta); err != nil {
		t.Fatal(err)
	}
	fx.stderr.Reset()
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--docs-vectors", docsDir, "--out", out), fx.e); code != 1 {
		t.Errorf("mismatched dimension: exit %d %s", code, fx.stderr.String())
	}
}

func TestEvaluateVectorsKeyMismatch(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	out := filepath.Join(fx.root, "vout")
	docs, qids := vecFile(t, o.dir, "docs.keys"), vecFile(t, o.dir, "queries.ids")
	for _, c := range []struct {
		name, file, content, want string
	}{
		{"docs missing a key", "docs.keys", strings.Join(strings.Split(docs, "\n")[1:], "\n"), "docs"},
		{"docs renamed key", "docs.keys", strings.Replace(docs, "reference_bs3pro", "reference_bs3pro_x", 1), "reference_bs3pro"},
		{"queries renamed id", "queries.ids", strings.Replace(qids, "case:cooler", "case:other", 1), "queries"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(o.dir, c.file)
			orig, _ := os.ReadFile(path)
			if err := os.WriteFile(path, []byte(c.content), 0o600); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.WriteFile(path, orig, 0o600) }()
			fx.stderr.Reset()
			code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out), fx.e)
			if code == 0 || !strings.Contains(fx.stderr.String(), c.want) {
				t.Errorf("exit %d: %s", code, fx.stderr.String())
			}
		})
	}
	fx.stderr.Reset()
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out, "--vec-weight", "-1"), fx.e); code != 2 {
		t.Errorf("negative weight: %d", code)
	}
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out, "--export-texts", out), fx.e); code != 2 {
		t.Errorf("export together with vectors: %d", code)
	}
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--docs-vectors", o.dir, "--out", out), fx.e); code != 2 {
		t.Errorf("--docs-vectors alone: %d", code)
	}
}

func vecFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestVecCrossFit(t *testing.T) {
	row := func(cos float64, r0, f0, r1, f1 float64) VecRow {
		return VecRow{VecSetting: VecSetting{Source: "vector", Take: 1, MinCos: cos}, Folds: [2]Fold{{Recall: r0, FalseRate: f0}, {Recall: r1, FalseRate: f1}},
			Gold: HookStats{Recall: (r0 + r1) / 2}, Negative: HookStats{InjectRate: (f0 + f1) / 2}}
	}
	rows := []VecRow{
		row(0.1, 0.30, 0.90, 0.30, 0.90),
		row(0.2, 0.25, 0.20, 0.10, 0.40),
		row(0.3, 0.15, 0.25, 0.20, 0.10),
		row(0.4, 0.05, 0.05, 0.05, 0.05),
	}
	markVecFrontier(rows)
	var k0, k1 []float64
	for _, r := range rows {
		if r.FoldKnee[0] {
			k0 = append(k0, r.MinCos)
		}
		if r.FoldKnee[1] {
			k1 = append(k1, r.MinCos)
		}
	}
	if !slices.Equal(k0, []float64{0.2}) || !slices.Equal(k1, []float64{0.3}) {
		t.Fatalf("fold knees %v %v", k0, k1)
	}
	cf := crossFitVec(rows)
	if len(cf) != 2 {
		t.Fatalf("%+v", cf)
	}
	a, b := cf[0], cf[1]
	if a.PickedOn != 0 || a.MinCos != 0.2 || a.HeldOut != rows[1].Folds[1] || a.HeldKnee.MinCos != 0.3 || a.HeldAtFIR != 0.20 {
		t.Errorf("pick on fold 0: %+v", a)
	}
	if b.PickedOn != 1 || b.MinCos != 0.3 || b.HeldOut != rows[2].Folds[0] || b.HeldKnee.MinCos != 0.2 || b.HeldAtFIR != 0.25 {
		t.Errorf("pick on fold 1: %+v", b)
	}
}

func TestEvaluateVectorsDocsFromAnotherModel(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	docsDir := filepath.Join(fx.root, "docs-run")
	if err := os.MkdirAll(docsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docs.f32", "docs.keys", "meta.json"} {
		data, err := os.ReadFile(filepath.Join(o.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(docsDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := vec.ReadMeta(docsDir)
	if err != nil {
		t.Fatal(err)
	}
	meta.Model = "another/model"
	if err := vec.WriteMeta(docsDir, meta); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(fx.root, "vout")
	code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--docs-vectors", docsDir, "--out", out), fx.e)
	if code != 1 || !strings.Contains(fx.stderr.String(), "model") {
		t.Errorf("documents from another model: exit %d %s", code, fx.stderr.String())
	}
}

func TestEvaluateVectorsRejectsNaNWeight(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	out := filepath.Join(fx.root, "vout")
	for _, w := range []string{"NaN", "+Inf"} {
		if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out, "--vec-weight", w), fx.e); code != 2 {
			t.Errorf("--vec-weight %s: exit %d", w, code)
		}
	}
}

func TestEvaluateVectorsRejectsVectorsFromAnotherExport(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	sha := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	meta, err := vec.ReadMeta(o.dir)
	if err != nil {
		t.Fatal(err)
	}
	meta.Queries = &vec.PartMeta{InputSHA256: sha(filepath.Join(o.export, QueriesFile))}
	meta.Docs = &vec.PartMeta{InputSHA256: sha(filepath.Join(o.export, DocsFile))}
	write := func() {
		if err := vec.WriteMeta(o.dir, meta); err != nil {
			t.Fatal(err)
		}
	}
	write()
	out := filepath.Join(fx.root, "vout")
	run := func(extra ...string) int {
		fx.stderr.Reset()
		return Run(fx.args(append([]string{"--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out}, extra...)...), fx.e)
	}
	if code := run(); code != 0 {
		t.Fatalf("vectors of this export: exit %d %s", code, fx.stderr.String())
	}
	meta.Queries.InputSHA256 = strings.Repeat("0", 64)
	write()
	if code := run(); code != 1 || !strings.Contains(fx.stderr.String(), "query vectors") || !strings.Contains(fx.stderr.String(), "different export") {
		t.Errorf("stale query vectors: exit %d %s", code, fx.stderr.String())
	}
	if code := run("--allow-other-texts"); code != 0 {
		t.Errorf("--allow-other-texts: exit %d %s", code, fx.stderr.String())
	}
	meta.Queries.InputSHA256 = sha(filepath.Join(o.export, QueriesFile))
	meta.Docs.InputSHA256 = strings.Repeat("0", 64)
	write()
	if code := run(); code != 1 || !strings.Contains(fx.stderr.String(), "document vectors") {
		t.Errorf("stale document vectors: exit %d %s", code, fx.stderr.String())
	}
}

func TestEvaluateVectorsAcceptsEmbedOutputOfItsOwnExport(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i, s := range req.Input {
			sum := sha256.Sum256([]byte(s))
			v := make([]float64, 8)
			for j := range v {
				v[j] = float64(sum[j]) + 1
			}
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	run := filepath.Join(fx.root, "embedded")
	if _, err := vec.Embed(t.Context(), vec.Options{URL: srv.URL, Model: "fake", In: o.export, Out: run, Which: "both", Batch: 4, Par: 2, Retries: 0}, nil); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(fx.root, "vout")
	if code := Run(fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", run, "--out", out), fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
}

func TestVectorQueriesCarryTopFourCounts(t *testing.T) {
	fx := newEvalFixture(t)
	o := buildOracle(t, fx, 16)
	out := filepath.Join(fx.root, "vout4")
	args := fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--vectors", o.dir, "--out", out)
	if code := Run(args, fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
	sum := loadVectorSummary(t, out)
	data, err := os.ReadFile(filepath.Join(out, "vector-queries.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	col := map[string]int{}
	for i, name := range strings.Split(lines[0], "\t") {
		col[name] = i
	}
	for _, name := range []string{"gold_n", "lexical_n4", "vector_n4", "fused_n4", "vector_all_n4"} {
		if _, ok := col[name]; !ok {
			t.Fatalf("no column %s in %q", name, lines[0])
		}
	}
	sums := map[string]float64{}
	gold := 0
	for _, line := range lines[1:] {
		c := strings.Split(line, "\t")
		if c[col["set"]] != SetGold {
			continue
		}
		gold++
		n, err := strconv.Atoi(c[col["gold_n"]])
		if err != nil || n == 0 {
			t.Fatalf("gold_n in %q", line)
		}
		for _, src := range []string{"lexical", "vector", "fused", "vector_all"} {
			k, err := strconv.Atoi(c[col[src+"_n4"]])
			if err != nil || k < 0 || k > n || k > 4 {
				t.Fatalf("%s_n4 in %q", src, line)
			}
			sums[src] += float64(k) / float64(n)
		}
	}
	if gold != sum.Gold {
		t.Fatalf("gold rows %d, summary %d", gold, sum.Gold)
	}
	for src, name := range map[string]string{"lexical": "lexical", "vector": "vector", "fused": "fused", "vector_all": "vector-all"} {
		want := metricOf(t, sum, name, "read").Recall4
		if got := sums[src] / float64(gold); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: mean of per-prompt n4/gold_n %.9f, summary R@4 %.9f", src, got, want)
		}
	}
}

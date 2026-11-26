package store

import (
	"context"
	"slices"
	"testing"
)

func vecSeed(t *testing.T) *Store {
	t.Helper()
	s, _ := newStore(t)
	mustPut(t, s, spec{key: "r-rule", title: "Rule title", body: "rule body about pushing", scopes: []string{"global"}}.req())
	mustPut(t, s, spec{key: "r-host", title: "Host fact", body: "fact body about a host", kind: Fact, scopes: []string{"host:smok"}}.req())
	mustPut(t, s, spec{key: "r-log#1", title: "", body: "log chunk about journal", kind: Log, scopes: []string{"global"}}.req())
	return s
}

func rowsFor(t *testing.T, s *Store, model, prefix string, keys ...string) []VecRow {
	t.Helper()
	var out []VecRow
	for _, k := range keys {
		r, err := s.Get(context.Background(), k)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, VecRow{Key: k, Vec: []float32{1, 0, 0}, TextSHA: TextSHA(DocText(prefix, r))})
	}
	return out
}

func TestVectorsSchema(t *testing.T) {
	s, _ := newStore(t)
	v, err := s.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Errorf("schema version %d, want %d", v, len(migrations))
	}
}

func TestPutVectorsAndStatus(t *testing.T) {
	ctx := context.Background()
	s := vecSeed(t)
	rows := rowsFor(t, s, "m", "D: ", "r-rule", "r-host", "r-log#1")
	n, skipped, err := s.PutVectors(ctx, "m", "D: ", rows)
	if err != nil || n != 3 || skipped != 0 {
		t.Fatalf("put: %d written, %d skipped, %v", n, skipped, err)
	}
	got, err := s.VectorInfo(ctx, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["r-rule"].SHA != rows[0].TextSHA || got["r-rule"].Dim != 3 {
		t.Errorf("info %+v", got)
	}
	other, err := s.VectorInfo(ctx, "other")
	if err != nil || len(other) != 0 {
		t.Errorf("other model has %d rows, %v", len(other), err)
	}
	models, err := s.VectorModels(ctx)
	if err != nil || len(models) != 1 || models[0].Model != "m" || models[0].Dim != 3 || models[0].Rows != 3 {
		t.Errorf("models %+v, %v", models, err)
	}
	if n, _, err := s.PutVectors(ctx, "m", "D: ", rows[:1]); err != nil || n != 1 {
		t.Errorf("upsert: %d, %v", n, err)
	}
	if got, _ := s.VectorInfo(ctx, "m"); len(got) != 3 {
		t.Errorf("upsert duplicated rows: %d", len(got))
	}
}

func TestPutVectorsSkipsChangedText(t *testing.T) {
	ctx := context.Background()
	s := vecSeed(t)
	rows := rowsFor(t, s, "m", "D: ", "r-rule")
	body := "rule body changed after the text was read"
	mustPut(t, s, PutRequest{Key: "r-rule", Update: true, Body: &body, Gate: DefaultGate(), Scopes: []string{"global"}, Kind: Rule})
	n, skipped, err := s.PutVectors(ctx, "m", "D: ", rows)
	if err != nil || n != 0 || skipped != 1 {
		t.Fatalf("%d written, %d skipped, %v", n, skipped, err)
	}
	if got, _ := s.VectorInfo(ctx, "m"); len(got) != 0 {
		t.Errorf("stale vector stored: %+v", got)
	}
}

func TestEditingTextDropsVector(t *testing.T) {
	ctx := context.Background()
	s := vecSeed(t)
	if _, _, err := s.PutVectors(ctx, "m", "D: ", rowsFor(t, s, "m", "D: ", "r-rule", "r-host")); err != nil {
		t.Fatal(err)
	}
	body := "rule body, edited"
	mustPut(t, s, PutRequest{Key: "r-rule", Update: true, Body: &body, Gate: DefaultGate(), Scopes: []string{"global"}, Kind: Rule})
	got, _ := s.VectorInfo(ctx, "m")
	if _, ok := got["r-rule"]; ok {
		t.Error("vector of an edited record survived")
	}
	if _, ok := got["r-host"]; !ok {
		t.Error("vector of an untouched record was dropped")
	}
	core := true
	mustPut(t, s, PutRequest{Key: "r-host", Update: true, Core: &core, Gate: DefaultGate(), Scopes: []string{"host:smok"}, Kind: Fact})
	if got, _ := s.VectorInfo(ctx, "m"); len(got) != 1 {
		t.Errorf("a change that leaves the text alone dropped the vector: %d rows", len(got))
	}
}

func TestScanVectors(t *testing.T) {
	ctx := context.Background()
	s := vecSeed(t)
	if _, _, err := s.PutVectors(ctx, "m", "", rowsFor(t, s, "m", "", "r-rule", "r-host", "r-log#1")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PutVectors(ctx, "o", "", rowsFor(t, s, "o", "", "r-rule")); err != nil {
		t.Fatal(err)
	}
	scan := func(model string, q Query) []string {
		var out []string
		err := s.ScanVectors(ctx, model, q, func(v VecScan) error {
			if v.Dim != 3 || len(v.Blob) != 12 {
				t.Errorf("%s: dim %d, blob %d", v.Key, v.Dim, len(v.Blob))
			}
			out = append(out, v.Key)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(out)
		return out
	}
	cases := []struct {
		name  string
		model string
		q     Query
		want  []string
	}{
		{"all current", "m", Query{}, []string{"r-host", "r-log#1", "r-rule"}},
		{"other model", "o", Query{}, []string{"r-rule"}},
		{"no logs", "m", Query{ExcludeKinds: []Kind{Log}}, []string{"r-host", "r-rule"}},
		{"scopes", "m", Query{Scopes: []string{"global"}}, []string{"r-log#1", "r-rule"}},
		{"excluded keys", "m", Query{ExcludeKeys: []string{"r-rule"}}, []string{"r-host", "r-log#1"}},
		{"unknown model", "x", Query{}, nil},
	}
	for _, c := range cases {
		if got := scan(c.model, c.q); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if _, err := s.Retire(ctx, "r-host", "gone"); err != nil {
		t.Fatal(err)
	}
	if got := scan("m", Query{}); !slices.Equal(got, []string{"r-log#1", "r-rule"}) {
		t.Errorf("retired record scanned: %v", got)
	}
}

func TestDocText(t *testing.T) {
	r := Record{Title: "T", Body: "B"}
	if got := DocText("Document: ", r); got != "Document: T\n\nB" {
		t.Errorf("%q", got)
	}
	if TextSHA("a") == TextSHA("b") || len(TextSHA("a")) != 64 {
		t.Error("sha")
	}
}

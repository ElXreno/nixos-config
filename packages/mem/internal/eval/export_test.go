package eval

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mem/internal/store"
)

func readJSONL[T any](t *testing.T, path string) []T {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExportTexts(t *testing.T) {
	fx := newEvalFixture(t)
	writeFiles(t, fx.root, map[string]string{
		"cases.json": `[{"name": "cooler", "prompt": "What does the 0xDF command do to the BS3 Pro cooler?", "want": ["reference_bs3pro"]},
			{"name": "unrelated", "prompt": "write a haiku about autumn leaves falling in the park", "none": true}]`,
	})
	dir := filepath.Join(fx.root, "export")
	args := fx.args("--cases", filepath.Join(fx.root, "cases.json"), "--export-texts", dir, "--out", filepath.Join(fx.root, "work"))
	if code := Run(args, fx.e); code != 0 {
		t.Fatalf("exit %d: %s", code, fx.stderr.String())
	}
	for _, leak := range []string{longMarker, "git push the agent-guard", "zfs readahead batch patches"} {
		if strings.Contains(fx.stdout.String(), leak) {
			t.Errorf("stdout leaks text %q", leak)
		}
	}
	for _, name := range []string{"queries.jsonl", "docs.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
	qs := readJSONL[QueryText](t, filepath.Join(dir, "queries.jsonl"))
	count := map[string]int{}
	ids := map[string]bool{}
	for _, q := range qs {
		count[q.Set]++
		if q.ID == "" || ids[q.ID] || q.Text == "" || q.Session == "" {
			t.Errorf("bad row %q %q", q.ID, q.Set)
		}
		ids[q.ID] = true
	}
	if count[SetGold] != 5 || count[SetNegative] != 2 || count[SetCase] != 2 || len(qs) != 9 {
		t.Fatalf("sets %v", count)
	}
	p, err := prepare(t.Context(), fx.e, Config{Glob: filepath.Join(fx.projects, "*.jsonl"), LegacyDir: fx.mem, Project: "repo", Out: filepath.Join(fx.root, "p"),
		Limit: DefaultLimit, Depth: DefaultDepth, Tail: DefaultTail, MinChars: DefaultMinChars, MaxQuery: DefaultMaxQuery, Seed: DefaultSeed,
		Cases: filepath.Join(fx.root, "cases.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.s.Close() }()
	byID := map[string]QueryText{}
	for _, q := range qs {
		byID[q.ID] = q
	}
	short, long := 0, 0
	for _, q := range append(append(p.qs[:len(p.qs):len(p.qs)], p.negs...), p.cases...) {
		got, ok := byID[p.id(q)]
		if !ok || got.Text != q.Query || got.Session != q.Session {
			t.Errorf("query %s: exported %q, hook sees %q", p.id(q), got.Text, q.Query)
		}
		if q.ShortTail && strings.Contains(got.Text, "zfs readahead batch patches") && got.Text != q.Text {
			short++
		}
		if strings.Contains(got.Text, longMarker) {
			long++
		}
	}
	if short == 0 || long == 0 {
		t.Errorf("short prompts with the assistant tail %d, long prompts %d", short, long)
	}
	docs := readJSONL[DocText](t, filepath.Join(dir, "docs.jsonl"))
	recs, err := p.s.List(t.Context(), store.Filter{AllStatus: true})
	if err != nil || len(docs) != len(recs) || len(docs) < 4 {
		t.Fatalf("docs %d records %d %v", len(docs), len(recs), err)
	}
	want := map[string]string{}
	for _, r := range recs {
		want[r.Key] = r.Title + "\n\n" + r.Body
	}
	for _, d := range docs {
		if want[d.Key] != d.Text {
			t.Errorf("doc %s text differs from title + blank line + body", d.Key)
		}
		delete(want, d.Key)
	}
	if len(want) != 0 {
		t.Errorf("records missing from docs.jsonl: %d", len(want))
	}
	if !strings.Contains(fx.stdout.String(), "queries 9 (gold 5, negative 2, case 2)") || !strings.Contains(fx.stdout.String(), "docs "+strconv.Itoa(len(recs))) {
		t.Errorf("counts line missing:\n%s", fx.stdout.String())
	}
	fx.stderr.Reset()
	if code := Run(fx.args("--export-texts", dir), fx.e); code != 2 || !strings.Contains(fx.stderr.String(), "--legacy-dir and --out are required") {
		t.Errorf("neither --out nor --db: exit %d %s", code, fx.stderr.String())
	}
	fx.stderr.Reset()
	if code := Run(fx.args("--export-texts", dir, "--out", filepath.Join(fx.root, "work"), "--cases", filepath.Join(fx.root, "none.json")), fx.e); code != 1 {
		t.Errorf("missing cases file: exit %d", code)
	}
}

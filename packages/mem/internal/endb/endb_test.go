package endb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

type rec struct{ key, title, body, kind string }

var russian = []rec{
	{"reference_cooler", "Кулер Flydigi", "Команда 0xDF прошивает кулер и превращает его в кирпич, отправлять нельзя.", "fact"},
	{"feedback_push", "Не пушить без спроса", "После коммита ждать явного разрешения пользователя, каждый пуш отдельное действие.", "rule"},
	{"project_saga", "Сага ZFS", "Readahead батчи для ZFS, ревью патчей и замеры на стенде.", "log"},
}

var english = map[string][2]string{
	"reference_cooler": {"Flydigi cooler", "The 0xDF command flashes the cooler and bricks it, never send it."},
	"feedback_push":    {"Do not push unasked", "After a commit wait for explicit user permission, every push is its own action."},
	"project_saga":     {"ZFS saga", "Readahead batches for ZFS, patch review and stand measurements."},
}

func fixtureDB(t *testing.T, dir, name string, recs []rec, now time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	s, err := store.Open(t.Context(), path, store.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		title, body := r.title, r.body
		_, err := s.Put(t.Context(), store.PutRequest{Key: r.key, Kind: store.Kind(r.kind), Scopes: []string{store.GlobalScope}, Title: &title, Body: &body,
			Gate: store.Gate{Dup: 2, Rel: 2, MinShared: 1000}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeEN(t *testing.T, path string, rows []map[string]any) {
	t.Helper()
	var b strings.Builder
	for _, r := range rows {
		data, _ := json.Marshal(r)
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func enRows() []map[string]any {
	var rows []map[string]any
	i := 0
	for _, r := range russian {
		i++
		e := english[r.key]
		row := map[string]any{"id": i, "key": r.key, "title_en": e[0], "body_en": e[1], "issues": nil}
		if i == 2 {
			row["issues"] = []string{"idiom"}
		}
		rows = append(rows, row)
	}
	return rows
}

func fileSum(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	in := fixtureDB(t, dir, "ru.db", russian, now)
	var enRecs []rec
	for _, r := range russian {
		e := english[r.key]
		enRecs = append(enRecs, rec{r.key, e[0], e[1], r.kind})
	}
	fresh := fixtureDB(t, dir, "fresh-en.db", enRecs, now)
	en := filepath.Join(dir, "en.jsonl")
	writeEN(t, en, enRows())
	before := fileSum(t, in)
	out := filepath.Join(dir, "ru-en.db")
	res, err := Build(t.Context(), Options{In: in, EN: en, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 3 || res.Replaced != 3 || res.Missing != 0 || res.WithIssues != 1 {
		t.Errorf("%+v", res)
	}
	if fileSum(t, in) != before {
		t.Error("the source database changed")
	}
	if info, err := os.Stat(out); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("output mode: %v %v", info, err)
	}
	a, err := store.Open(t.Context(), out, store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := store.Open(t.Context(), fresh, store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	src, err := store.Open(t.Context(), in, store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	for _, r := range russian {
		got, err := a.Get(t.Context(), r.key)
		if err != nil {
			t.Fatal(err)
		}
		old, _ := src.Get(t.Context(), r.key)
		want := english[r.key]
		if got.Title != want[0] || got.Body != want[1] {
			t.Errorf("%s: title %q body %q", r.key, got.Title, got.Body)
		}
		if got.Version != old.Version || !got.UpdatedAt.Equal(old.UpdatedAt) || got.Kind != old.Kind || !reflect.DeepEqual(got.Scopes, old.Scopes) {
			t.Errorf("%s: metadata changed: %+v vs %+v", r.key, got, old)
		}
		if h, err := a.History(t.Context(), r.key); err != nil || len(h) != 0 {
			t.Errorf("%s: history %v %v", r.key, h, err)
		}
	}
	for _, q := range []string{"cooler bricks flashes", "explicit permission push", "readahead zfs patch", "0xDF", "кулер", "пуш разрешения", "stand measurements"} {
		for _, mode := range []store.Mode{store.Fused, store.WordsOnly, store.TrigramOnly} {
			qq := store.Query{Text: q, Limit: 10, Mode: mode, AllStatus: true}
			ga, err1 := a.Search(t.Context(), qq)
			gb, err2 := b.Search(t.Context(), qq)
			if err1 != nil || err2 != nil {
				t.Fatal(err1, err2)
			}
			if len(ga) != len(gb) {
				t.Errorf("%q %s: %d hits against %d from a fresh English database", q, mode, len(ga), len(gb))
				continue
			}
			for i := range ga {
				x, y := ga[i], gb[i]
				if x.Record.Key != y.Record.Key || x.Score != y.Score || x.WordsRank != y.WordsRank || x.TriRank != y.TriRank || x.WordsBM25 != y.WordsBM25 {
					t.Errorf("%q %s rank %d: %s %v/%v/%v vs %s %v/%v/%v", q, mode, i+1, x.Record.Key, x.Score, x.WordsRank, x.TriRank, y.Record.Key, y.Score, y.WordsRank, y.TriRank)
				}
			}
		}
	}
	if hits, _ := a.Search(t.Context(), store.Query{Text: "кулер", Mode: store.WordsOnly, AllStatus: true}); len(hits) != 0 {
		t.Errorf("Russian words still indexed: %d hits", len(hits))
	}
	if hits, _ := a.Search(t.Context(), store.Query{Text: "bricks", Mode: store.WordsOnly, AllStatus: true}); len(hits) != 1 || hits[0].Record.Key != "reference_cooler" {
		t.Errorf("English word not indexed: %v", hits)
	}
}

func TestBuildErrors(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	in := fixtureDB(t, dir, "ru.db", russian, now)
	run := func(name string, mod func([]map[string]any) []map[string]any, o Options) error {
		en := filepath.Join(dir, name+".jsonl")
		writeEN(t, en, mod(enRows()))
		o.In, o.EN = in, en
		if o.Out == "" {
			o.Out = filepath.Join(dir, name+".db")
		}
		_, err := Build(context.Background(), o)
		if err == nil {
			return nil
		}
		if _, statErr := os.Stat(o.Out); statErr == nil && !strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("%w (output left behind)", err)
		}
		return err
	}
	for _, c := range []struct {
		name string
		mod  func([]map[string]any) []map[string]any
		o    Options
		want string
	}{
		{"unknown", func(r []map[string]any) []map[string]any { r[0]["key"] = "nope"; return r }, Options{}, "nope"},
		{"duplicate", func(r []map[string]any) []map[string]any { r[1]["key"] = r[0]["key"]; return r }, Options{}, "duplicate"},
		{"empty-body", func(r []map[string]any) []map[string]any { r[0]["body_en"] = "  "; return r }, Options{}, "empty"},
		{"empty-title", func(r []map[string]any) []map[string]any { r[0]["title_en"] = " "; return r }, Options{}, "title"},
		{"missing", func(r []map[string]any) []map[string]any { return r[:2] }, Options{}, "missing"},
		{"same", func(r []map[string]any) []map[string]any { return r }, Options{Out: in}, "same"},
	} {
		if err := run(c.name, c.mod, c.o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	res, err := Build(t.Context(), Options{In: in, EN: filepath.Join(dir, "nowhere.jsonl"), Out: filepath.Join(dir, "partial.db"), AllowMissing: true})
	if err == nil {
		t.Errorf("missing en file accepted: %+v", res)
	}
	if err := run("partial", func(r []map[string]any) []map[string]any { return r[:2] }, Options{AllowMissing: true}); err != nil {
		t.Errorf("--allow-missing: %v", err)
	}
	out := filepath.Join(dir, "exists.db")
	if err := os.WriteFile(out, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run("exists", func(r []map[string]any) []map[string]any { return r }, Options{Out: out}); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("existing output: %v", err)
	}
	if err := run("force", func(r []map[string]any) []map[string]any { return r }, Options{Out: out, Force: true}); err != nil {
		t.Errorf("--force: %v", err)
	}
}

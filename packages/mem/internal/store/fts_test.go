package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFTS5Available(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "fts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	var version string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("sqlite %s", version)

	rows, err := db.QueryContext(ctx, "PRAGMA compile_options")
	if err != nil {
		t.Fatal(err)
	}
	var opts []string
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			t.Fatal(err)
		}
		opts = append(opts, o)
	}
	_ = rows.Close()
	if !slices.Contains(opts, "ENABLE_FTS5") {
		t.Fatalf("ENABLE_FTS5 missing from compile options %v", opts)
	}

	for _, stmt := range []string{
		"CREATE VIRTUAL TABLE w USING fts5(body, tokenize = 'unicode61 remove_diacritics 2')",
		"CREATE VIRTUAL TABLE g USING fts5(body, tokenize = 'trigram remove_diacritics 1')",
		"INSERT INTO w (rowid, body) VALUES (1, 'Сборка ядра на СМОК'), (2, 'never write 0xDF to the cooler'), (3, 'café ёлка')",
		"INSERT INTO g (rowid, body) VALUES (1, 'Сборка ядра на СМОК'), (2, 'never write cmd0xDF to the cooler'), (3, 'curl 127.0.0.1:8097/v1')",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	cases := []struct {
		table, match string
		want         []int
	}{
		{"w", `"сборка"`, []int{1}},
		{"w", `"СБОРК"*`, []int{1}},
		{"w", `"смок"`, []int{1}},
		{"w", `"0xdf"`, []int{2}},
		{"w", `"cafe"`, []int{3}},
		{"w", `"ёлка"`, []int{3}},
		{"w", `"елка"`, nil},
		{"g", `"0xdf"`, []int{2}},
		{"g", `"борк"`, []int{1}},
		{"g", `"0.1:809"`, []int{3}},
		{"g", `"смок" OR "cooler"`, []int{1, 2}},
	}
	for _, c := range cases {
		rows, err := db.QueryContext(ctx, "SELECT rowid FROM "+c.table+" WHERE "+c.table+" MATCH ? ORDER BY rowid", c.match)
		if err != nil {
			t.Fatalf("%s MATCH %s: %v", c.table, c.match, err)
		}
		var got []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			got = append(got, id)
		}
		_ = rows.Close()
		if !slices.Equal(got, c.want) {
			t.Errorf("%s MATCH %s = %v, want %v", c.table, c.match, got, c.want)
		}
	}

	var score float64
	if err := db.QueryRowContext(ctx, `SELECT bm25(w) FROM w WHERE w MATCH '"сборка"'`).Scan(&score); err != nil {
		t.Fatalf("bm25: %v", err)
	}
	if score >= 0 {
		t.Errorf("bm25 = %v, want negative", score)
	}
}

func TestQueryTermsFallBackOnlyWhenAsked(t *testing.T) {
	cases := []struct {
		in       string
		fallback bool
		words    string
	}{
		{"нельзя", false, ""},
		{"нельзя", true, `"нельз"*`},
		{"никогда нельзя", true, `"никогд"* OR "нельз"*`},
		{"must only", true, `"must"* OR "onl"*`},
		{"never push", true, `"push"*`},
		{"и в на", false, ""},
		{"и в на нельзя", true, `"нельз"*`},
		{"и в на", true, `"и" OR "в" OR "на"`},
		{"the of", true, `"the"* OR "of"`},
		{"push the remote", true, `"push"* OR "remot"*`},
		{"", true, ""},
	}
	for _, c := range cases {
		q := Query{Text: c.in, Fallback: c.fallback}
		if got := wordsQuery(q.terms()); got != c.words {
			t.Errorf("words query of %q (fallback %v) = %q, want %q", c.in, c.fallback, got, c.words)
		}
		if c.fallback {
			continue
		}
		if got := WordsQuery(c.in); got != c.words {
			t.Errorf("WordsQuery(%q) = %q, want %q", c.in, got, c.words)
		}
	}
	if got := TrigramQuery("и в на", true); got != "" {
		t.Errorf("TrigramQuery of two-letter words = %q, want empty", got)
	}
	if got := TrigramQuery("никогда", true); got != "" {
		t.Errorf("TrigramQuery(никогда) = %q, want empty", got)
	}
	if got := trigramQuery("никогда", Query{Text: "никогда", Fallback: true}.terms(), true); got != `"никогд"` {
		t.Errorf("fallback trigram query of никогда = %q", got)
	}
}

func TestSearchFindsNegationOnlyQuery(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	a := spec{key: "no_push", title: "Push policy", body: "Нельзя пушить в nixos-config без подтверждения."}
	b := spec{key: "zfs_arc", title: "ZFS ARC", body: "Лимит ARC задаётся при загрузке."}
	mustPut(t, s, a.req())
	mustPut(t, s, b.req())
	for _, mode := range []Mode{Fused, WordsOnly, TrigramOnly} {
		for _, qs := range []string{"нельзя", "никогда нельзя"} {
			hits, err := s.Search(ctx, Query{Text: qs, Mode: mode, Fallback: true})
			if err != nil {
				t.Fatal(err)
			}
			if got := keys(hits); !slices.Equal(got, []string{a.key}) {
				t.Errorf("mode %v query %q = %v, want %s", mode, qs, got, a.key)
			}
		}
	}
	hits, err := s.Search(ctx, Query{Text: "нельзя", Mode: WordsOnly, Fallback: true})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits %v err %v", hits, err)
	}
	if hits[0].Overlap != 1 {
		t.Errorf("overlap = %v, want 1", hits[0].Overlap)
	}
	if !strings.Contains(strings.ToLower(hits[0].Snippet), "нельзя") {
		t.Errorf("snippet = %q", hits[0].Snippet)
	}
}

func TestSearchFindsModalOnlyQuery(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	mustPut(t, s, spec{key: "deploy_rule", title: "Deploy", body: "Always confirm before a switch."}.req())
	mustPut(t, s, spec{key: "zfs_arc", title: "ZFS ARC", body: "ARC is capped at boot."}.req())
	for _, qs := range []string{"always", "must always"} {
		hits, err := s.Search(ctx, Query{Text: qs, Mode: WordsOnly, Fallback: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(hits); !slices.Equal(got, []string{"deploy_rule"}) {
			t.Errorf("query %q = %v, want [deploy_rule]", qs, got)
		}
	}
}

func TestSearchOfFunctionWordsOnlyNeedsFallback(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	mustPut(t, s, spec{key: "pick_the", title: "Pick the one", body: "Take the red one of the two."}.req())
	mustPut(t, s, spec{key: "other", title: "Other", body: "Nothing relevant here at all."}.req())
	strict, err := s.Search(ctx, Query{Text: "the of", Mode: WordsOnly})
	if err != nil || len(strict) != 0 {
		t.Errorf("strict query of function words = %v err %v, want none", keys(strict), err)
	}
	hits, err := s.Search(ctx, Query{Text: "the of", Mode: WordsOnly, Fallback: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(hits); !slices.Equal(got, []string{"pick_the"}) {
		t.Errorf("fallback query = %v, want [pick_the]", got)
	}
	if hits[0].Overlap == 0 || hits[0].Snippet == "" {
		t.Errorf("fallback hit overlap %v snippet %q", hits[0].Overlap, hits[0].Snippet)
	}
	lw, _, err := s.Lists(ctx, Query{Text: "the of", Mode: WordsOnly, Fallback: true})
	if err != nil || len(lw) != 1 {
		t.Errorf("fallback lists = %v err %v", lw, err)
	}
	lw, _, err = s.Lists(ctx, Query{Text: "the of", Mode: WordsOnly})
	if err != nil || len(lw) != 0 {
		t.Errorf("strict lists = %v err %v", lw, err)
	}
}

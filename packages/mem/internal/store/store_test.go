package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func keys(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Record.Key
	}
	return out
}

func TestGateEscapes(t *testing.T) {
	ctx := context.Background()
	dupNew := spec{key: "never-push-unasked", title: "Never push without asking",
		body: "Never push to the remote by default: after committing, wait until the user explicitly authorizes the push."}
	cases := []struct {
		name    string
		old     spec
		mutate  func(*PutRequest)
		wantErr string
		check   func(t *testing.T, s *Store, res PutResult)
	}{
		{
			name:    "same scope duplicate is rejected",
			old:     enPush,
			wantErr: "gate",
		},
		{
			name: "distinct without why is invalid",
			old:  enPush,
			mutate: func(r *PutRequest) {
				r.Distinct = []string{enPush.key}
			},
			wantErr: "invalid",
		},
		{
			name: "distinct listing a different key is rejected",
			old:  enPush,
			mutate: func(r *PutRequest) {
				r.Distinct = []string{"other"}
				r.Why = "different"
			},
			wantErr: "gate",
		},
		{
			name: "distinct with why passes and is recorded",
			old:  enPush,
			mutate: func(r *PutRequest) {
				r.Distinct = []string{enPush.key}
				r.Why = "covers the remote, not the commit"
			},
			check: func(t *testing.T, s *Store, res PutResult) {
				h, err := s.History(ctx, dupNew.key)
				if err != nil || len(h) != 0 {
					t.Fatalf("history of a new record = %+v, %v", h, err)
				}
				links, _ := s.Links(ctx, dupNew.key)
				want := Link{Src: dupNew.key, Dst: enPush.key, Rel: Relates, Note: "distinct: covers the remote, not the commit"}
				if !slices.Contains(links, want) {
					t.Fatalf("links = %+v", links)
				}
				mustPut(t, s, PutRequest{Key: dupNew.key, Update: true, Body: ptr("Never push to the remote unasked."), Gate: DefaultGate()})
				if h, _ := s.History(ctx, dupNew.key); len(h) != 1 || h[0].Version != 1 || h[0].Reason != "update" {
					t.Fatalf("history after update = %+v", h)
				}
				if err := s.AddLinks(ctx, []Link{{Src: dupNew.key, Dst: enPush.key, Rel: Relates}}); err != nil {
					t.Fatal(err)
				}
				if links, _ := s.Links(ctx, dupNew.key); !slices.Contains(links, want) {
					t.Fatalf("a link without a note dropped the note: %+v", links)
				}
			},
		},
		{
			name: "supersedes skips the superseded record",
			old:  enPush,
			mutate: func(r *PutRequest) {
				r.Supersedes = []string{enPush.key}
			},
			check: func(t *testing.T, s *Store, res PutResult) {
				if !slices.Equal(res.Superseded, []string{enPush.key}) {
					t.Fatalf("superseded = %v", res.Superseded)
				}
			},
		},
		{
			name: "other project scope is not a neighbour",
			old:  spec{key: enPush.key, title: enPush.title, body: enPush.body, scopes: []string{"project:other"}},
			mutate: func(r *PutRequest) {
				r.Scopes = []string{"project:nixos-config"}
			},
		},
		{
			name: "global old record overlaps a project scope",
			old:  enPush,
			mutate: func(r *PutRequest) {
				r.Scopes = []string{"project:nixos-config"}
			},
			wantErr: "gate",
		},
		{
			name: "global new record overlaps every scope",
			old:  spec{key: enPush.key, title: enPush.title, body: enPush.body, scopes: []string{"host:smok"}},
			mutate: func(r *PutRequest) {
				r.Scopes = []string{GlobalScope}
			},
			wantErr: "gate",
		},
		{
			name: "other kind is not a neighbour",
			old:  spec{key: enPush.key, title: enPush.title, body: enPush.body, kind: Fact},
		},
		{
			name: "skip gate",
			old:  enPush,
			mutate: func(r *PutRequest) {
				r.Gate.Skip = true
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newStore(t)
			mustPut(t, s, c.old.req())
			req := dupNew.req()
			if c.mutate != nil {
				c.mutate(&req)
			}
			res, err := s.Put(ctx, req)
			var ge *GateError
			switch c.wantErr {
			case "gate":
				if !errors.As(err, &ge) {
					t.Fatalf("want GateError, got %v", err)
				}
				return
			case "invalid":
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("want ErrInvalid, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("put: %v", err)
			}
			if c.check != nil {
				c.check(t, s, res)
			}
		})
	}
}

func TestExistingKey(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	mustPut(t, s, enPush.req())
	_, err := s.Put(ctx, enPush.req())
	var ee *ExistsError
	if !errors.As(err, &ee) || !strings.Contains(err.Error(), "--update") {
		t.Fatalf("want ExistsError mentioning --update, got %v", err)
	}
	req := enPush.req()
	req.Key = "missing"
	req.Update = true
	if _, err := s.Put(ctx, req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of missing key: %v", err)
	}
}

func TestSupersedeChain(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	a := spec{key: "deploy-rule-a", title: "Build before deploy", body: "Always nix build before every switch."}
	b := spec{key: "deploy-rule-b", title: "Build before deploy, narrowed", body: "Build the toplevel once before the final switch."}
	d := spec{key: "deploy-rule-c", title: "Build once at the end", body: "Evaluate and build every host once at the end, then the user switches."}
	mustPut(t, s, a.req())
	c.advance(time.Hour)
	rb := b.req()
	rb.Supersedes = []string{a.key}
	rb.Why = "narrowed"
	mustPut(t, s, rb)
	c.advance(time.Hour)
	rc := d.req()
	rc.Supersedes = []string{b.key}
	mustPut(t, s, rc)

	want := []struct {
		key          string
		status       Status
		supersededBy string
		validTo      bool
		version      int
	}{
		{a.key, Superseded, b.key, true, 2},
		{b.key, Superseded, d.key, true, 2},
		{d.key, Current, "", false, 1},
	}
	for _, w := range want {
		r, err := s.Get(ctx, w.key)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != w.status || r.SupersededBy != w.supersededBy || r.ValidTo.IsZero() == w.validTo || r.Version != w.version {
			t.Errorf("%s = status %s by %q valid_to %v version %d", w.key, r.Status, r.SupersededBy, r.ValidTo, r.Version)
		}
	}
	links, _ := s.Links(ctx, b.key)
	wantLinks := []Link{{Src: b.key, Dst: a.key, Rel: Supersedes}, {Src: d.key, Dst: b.key, Rel: Supersedes}}
	if !slices.Equal(links, wantLinks) {
		t.Errorf("links = %+v, want %+v", links, wantLinks)
	}
	hits, err := s.Search(ctx, Query{Text: "build before deploy switch"})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(hits); !slices.Equal(got, []string{d.key}) {
		t.Errorf("current search = %v, want only %s", got, d.key)
	}
	hits, _ = s.Search(ctx, Query{Text: "build before deploy switch", AllStatus: true})
	if got := keys(hits); len(got) != 3 {
		t.Errorf("all-status search = %v, want 3", got)
	}
	req := a.req()
	req.Update = true
	var se *StatusError
	if _, err := s.Put(ctx, req); !errors.As(err, &se) || se.SupersededBy != b.key {
		t.Errorf("update of superseded record: %v", err)
	}
	rd := spec{key: "deploy-rule-d", body: "x"}.req()
	rd.Supersedes = []string{a.key}
	if _, err := s.Put(ctx, rd); !errors.As(err, &se) {
		t.Errorf("superseding a superseded record: %v", err)
	}
	h, _ := s.History(ctx, a.key)
	if len(h) != 1 || h[0].Status != Current || h[0].Reason != "superseded by "+b.key+": narrowed" {
		t.Errorf("history of a = %+v", h)
	}
}

func TestHistoryVersions(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	bodies := []string{"SMOK BIOS is 50D.", "SMOK BIOS is 50E.", "SMOK BIOS is 50E, newer than public 50D."}
	r := spec{key: "smok-bios", title: "SMOK BIOS", body: bodies[0], kind: State, scopes: []string{"host:SMOK"}}
	mustPut(t, s, r.req())
	for i, b := range bodies[1:] {
		c.advance(time.Hour)
		req := PutRequest{Key: r.key, Update: true, Body: ptr(b), Why: "bump " + string(rune('1'+i)), Gate: DefaultGate()}
		res := mustPut(t, s, req)
		if res.Version != i+2 {
			t.Fatalf("version = %d, want %d", res.Version, i+2)
		}
	}
	res := mustPut(t, s, PutRequest{Key: r.key, Update: true, Body: ptr(bodies[2]), Gate: DefaultGate()})
	if !res.Unchanged || res.Version != 3 {
		t.Fatalf("identical update = %+v", res)
	}
	got, err := s.Get(ctx, r.key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != bodies[2] || got.Version != 3 || !slices.Equal(got.Scopes, []string{"host:smok"}) || got.TTLDays != DefaultStateTTL {
		t.Fatalf("current = %+v", got)
	}
	if !got.UpdatedAt.Equal(c.t) || !got.CreatedAt.Equal(c.t.Add(-2*time.Hour)) {
		t.Errorf("created %v updated %v", got.CreatedAt, got.UpdatedAt)
	}
	h, err := s.History(ctx, r.key)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 2 {
		t.Fatalf("history len = %d", len(h))
	}
	for i, v := range h {
		if v.Version != i+1 || v.Body != bodies[i] || v.Reason != "update: bump "+string(rune('1'+i)) {
			t.Errorf("history[%d] = version %d body %q reason %q", i, v.Version, v.Body, v.Reason)
		}
		if !slices.Equal(v.Scopes, []string{"host:smok"}) {
			t.Errorf("history[%d] scopes = %v", i, v.Scopes)
		}
	}
	core := true
	res = mustPut(t, s, PutRequest{Key: r.key, Update: true, Core: &core, Gate: DefaultGate()})
	if res.Version != 4 {
		t.Fatalf("flag-only update version = %d", res.Version)
	}
	got, _ = s.Get(ctx, r.key)
	if got.Body != bodies[2] || !got.Core {
		t.Fatalf("flag-only update lost body or core: %+v", got)
	}
}

var corpus = []spec{
	{key: "no-push-default", title: "No git push by default", body: "Do not git push by default; wait for explicit authorization after every commit."},
	{key: "bs3pro-0xdf-bricks", title: "Flydigi BS3 Pro command 0xDF", body: "Never send command 0xDF to the BS3 Pro cooler: it bricks the device.", kind: Fact},
	{key: "freetoken-port", title: "FreeToken prod port", body: "The prod model listens on 127.0.0.1:8097; do not send requests there without approval.", kind: Fact},
	{key: "volume-hands-off", title: "Громкость не трогать", body: "Громкость через wpctl, pactl и amixer не менять без спроса."},
	{key: "builds-background", title: "Сборки в фоне", body: "Длинные сборки nix build запускать в фоне, не на батарее."},
	{key: "zfs-readahead-batch", title: "ZFS readahead patches", body: "Own readahead patches for ZFS run on SMOK in production.", kind: State, scopes: []string{"host:smok"}},
	{key: "kurwa-samsung-firmware", title: "Samsung firmware ban", body: "Never install Samsung firmware 1B2QNXH7 on any NVMe drive.", kind: Fact},
	{key: "tailscale-buffers", title: "Tailscale socket buffers", body: "Tailscale socketBufferSize is patched from 7 MB to 16 MB to stop UDP drops.", kind: Fact},
	{key: "deploy-log", title: "Deploy journal", body: "2026-10-01 deployed SMOK. 2026-10-02 switched KURWA.", kind: Log},
	{key: "ёлка-note", title: "", body: ""},
}

func seed(t *testing.T, s *Store) {
	t.Helper()
	for _, sp := range corpus {
		if sp.body == "" {
			continue
		}
		req := sp.req()
		req.Gate.Skip = true
		mustPut(t, s, req)
	}
}

func TestSearchRanking(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	seed(t, s)
	mustPut(t, s, PutRequest{Key: "yolka", Kind: Fact, Scopes: []string{GlobalScope}, Body: ptr("Ёлка стоит в углу."), Gate: Gate{Skip: true}})
	cases := []struct {
		name  string
		query Query
		top   string
		none  bool
	}{
		{name: "en words", query: Query{Text: "can I push the commit to the remote?"}, top: "no-push-default"},
		{name: "en morphology", query: Query{Text: "pushing commits"}, top: "no-push-default"},
		{name: "hex trigram", query: Query{Text: "what does 0xdf do on the cooler"}, top: "bs3pro-0xdf-bricks"},
		{name: "substring inside token", query: Query{Text: "BufferSize"}, top: "tailscale-buffers"},
		{name: "port", query: Query{Text: "curl 127.0.0.1:8097/v1/models"}, top: "freetoken-port"},
		{name: "ru morphology", query: Query{Text: "можно поменять громкость динамиков?"}, top: "volume-hands-off"},
		{name: "ru builds", query: Query{Text: "сборку запустить на батарее"}, top: "builds-background"},
		{name: "slug fragment", query: Query{Text: "readahead"}, top: "zfs-readahead-batch"},
		{name: "firmware id", query: Query{Text: "1B2QNXH7"}, top: "kurwa-samsung-firmware"},
		{name: "yo folding", query: Query{Text: "елка"}, top: "yolka"},
		{name: "yo query", query: Query{Text: "ёлка"}, top: "yolka"},
		{name: "kind filter", query: Query{Text: "deployed SMOK", ExcludeKinds: []Kind{Log}}, top: "zfs-readahead-batch"},
		{name: "kind include", query: Query{Text: "deployed SMOK", Kinds: []Kind{Log}}, top: "deploy-log"},
		{name: "scope filter", query: Query{Text: "readahead", Scopes: []string{"host:kurwa", GlobalScope}}, none: true},
		{name: "scope case", query: Query{Text: "readahead patches", Scopes: []string{"host:SMOK"}}, top: "zfs-readahead-batch"},
		{name: "exclude keys", query: Query{Text: "1B2QNXH7", ExcludeKeys: []string{"kurwa-samsung-firmware"}}, none: true},
		{name: "words only", query: Query{Text: "0xdf", Mode: WordsOnly}, top: "bs3pro-0xdf-bricks"},
		{name: "trigram only", query: Query{Text: "8097", Mode: TrigramOnly}, top: "freetoken-port"},
		{name: "nothing", query: Query{Text: "quantum chromodynamics"}, none: true},
		{name: "stopwords only", query: Query{Text: "the of и в"}, none: true},
		{name: "negation only is empty when strict", query: Query{Text: "never", Mode: WordsOnly}, none: true},
		{name: "negation only with fallback", query: Query{Text: "never", Mode: WordsOnly, Fallback: true}, top: "kurwa-samsung-firmware"},
		{name: "fts syntax is quoted", query: Query{Text: `push" OR NEAR(x) AND -"`}, top: "no-push-default"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hits, err := s.Search(ctx, c.query)
			if err != nil {
				t.Fatal(err)
			}
			if c.none {
				if len(hits) != 0 {
					t.Fatalf("hits = %v, want none", keys(hits))
				}
				return
			}
			if len(hits) == 0 || hits[0].Record.Key != c.top {
				t.Fatalf("hits = %v, want top %s", keys(hits), c.top)
			}
			h := hits[0]
			if h.Score <= 0 || h.Overlap <= 0 || h.Snippet == "" {
				t.Errorf("top hit = score %v overlap %v snippet %q", h.Score, h.Overlap, h.Snippet)
			}
			if c.query.Mode == WordsOnly && h.TriRank != 0 || c.query.Mode == TrigramOnly && h.WordsRank != 0 {
				t.Errorf("mode %s leaked ranks: words %d tri %d", c.query.Mode, h.WordsRank, h.TriRank)
			}
		})
	}
}

func TestSearchFusionPrefersAgreement(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	seed(t, s)
	hits, err := s.Search(ctx, Query{Text: "tailscale socketBufferSize udp", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Record.Key != "tailscale-buffers" || hits[0].WordsRank != 1 || hits[0].TriRank != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	if want := (1 + TriWeight) / float64(RRFK+1); math.Abs(hits[0].Score-want) > 1e-12 {
		t.Errorf("score = %v, want %v", hits[0].Score, want)
	}
	hits, err = s.Search(ctx, Query{Text: "tailscale socket buffers udp", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Record.Key != "tailscale-buffers" || hits[0].TriRank != 0 {
		t.Fatalf("plain words reached the trigram table: %+v", hits)
	}
}

func TestTrigramQuery(t *testing.T) {
	for _, c := range []struct {
		q     string
		stems bool
		want  string
	}{
		{"what does 0xDF do", false, `"0xdf"`},
		{"set socketBufferSize, then retry", false, `"socketbuffersize"`},
		{"curl 127.0.0.1:8097/v1 now", false, `"127.0.0.1:8097/v1"`},
		{"zfs_arc_max is unset", false, `"zfs_arc_max"`},
		{"Tailscale socket buffers", false, ""},
		{"Tailscale socket buffers", true, `"tailscal" OR "socket" OR "buffer"`},
		{"ёлка-2", false, `"елка-2"`},
	} {
		if got := TrigramQuery(c.q, c.stems); got != c.want {
			t.Errorf("TrigramQuery(%q, %v) = %s, want %s", c.q, c.stems, got, c.want)
		}
	}
}

func TestSearchCollapsesChunks(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	for i, b := range []string{"zfs arc grows on boot", "zfs arc shrinks under pressure", "zfs arc cap set"} {
		mustPut(t, s, PutRequest{Key: fmt.Sprintf("zfs-journal#%d", i+1), Kind: Log, Scopes: []string{GlobalScope}, Body: ptr(b), Gate: Gate{Skip: true}})
	}
	mustPut(t, s, PutRequest{Key: "zfs-arc-cap", Kind: Fact, Scopes: []string{GlobalScope}, Body: ptr("zfs arc cap is 8 GB"), Gate: Gate{Skip: true}})
	hits, err := s.Search(ctx, Query{Text: "zfs arc"})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(hits); len(got) != 2 || !slices.Contains(got, "zfs-arc-cap") || strings.Count(strings.Join(got, " "), "zfs-journal#") != 1 {
		t.Errorf("collapsed hits = %v, want one chunk and the fact", got)
	}
	hits, err = s.Search(ctx, Query{Text: "zfs arc", AllChunks: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(hits); len(got) != 4 {
		t.Errorf("all chunks = %v, want 4", got)
	}
}

func TestCoreRenderAndBudget(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	mustPut(t, s, spec{key: "b-rule", title: "Second rule.", body: "Line one.\nLine two.", core: true}.req())
	mustPut(t, s, spec{key: "a-state", title: "Pinned kernel", body: "Kernel pinned to 7.2.", kind: State, core: true}.req())
	mustPut(t, s, spec{key: "a-rule", body: "No title here.", core: true}.req())
	mustPut(t, s, spec{key: "host-only", body: "SMOK only core.", core: true, scopes: []string{"host:smok"}}.req())
	mustPut(t, s, spec{key: "not-core", body: "Not in core."}.req())
	out, err := s.RenderCore(ctx, CoreScopes, CoreBudget)
	if err != nil {
		t.Fatal(err)
	}
	want := coreHeader + "\n## Rules\n\n" +
		"- `a-rule`: No title here.\n" +
		"- `b-rule`: Second rule. Line one.\n  Line two.\n" +
		"\n## State\n\n" +
		"- `a-state` [as of 2026-11-07]: Pinned kernel. Kernel pinned to 7.2.\n"
	if out != want {
		t.Fatalf("core render:\n%s\nwant:\n%s", out, want)
	}

	path := filepath.Join(t.TempDir(), "sub", "memory-core.md")
	n, err := s.WriteCore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != want || n != len([]rune(want)) {
		t.Fatalf("written core differs: %d chars", n)
	}

	room := CoreBudget - (len([]rune(want)) - len([]rune(coreHeader)))
	fill := strings.Repeat("я", room-len([]rune("- `fill`: \n"))-len([]rune("\n## Facts\n\n")))
	mustPut(t, s, spec{key: "fill", body: fill, kind: Fact, core: true}.req())
	out, err = s.RenderCore(ctx, CoreScopes, CoreBudget)
	if err != nil || len([]rune(out))-len([]rune(coreHeader)) != CoreBudget {
		t.Fatalf("exact budget: %d chars, %v", len([]rune(out))-len([]rune(coreHeader)), err)
	}
	_, err = s.Put(ctx, spec{key: "overflow", body: "one more", kind: Howto, core: true}.req())
	var be *BudgetError
	if !errors.As(err, &be) || be.Size <= CoreBudget {
		t.Fatalf("over budget put: %v", err)
	}
	if _, err := s.Get(ctx, "overflow"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("over budget record was stored: %v", err)
	}
	mustPut(t, s, spec{key: "overflow", body: "one more", kind: Howto}.req())
	_, err = s.Put(ctx, PutRequest{Key: "overflow", Update: true, Core: ptr(true)})
	if !errors.As(err, &be) {
		t.Fatalf("over budget update: %v", err)
	}
	got, _ := s.Get(ctx, "overflow")
	if got.Core || got.Version != 1 {
		t.Fatalf("over budget update was applied: %+v", got)
	}
	mustPut(t, s, spec{key: "scoped-big", body: strings.Repeat("x", CoreBudget), core: true, scopes: []string{"project:p"}}.req())
}

func TestExpire(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	mustPut(t, s, spec{key: "pushed-state", body: "agent-guard refactor is not pushed.", kind: State}.req())
	ttl := 3
	mustPut(t, s, PutRequest{Key: "short-state", Kind: State, Scopes: []string{GlobalScope}, Body: ptr("short"), TTLDays: &ttl, Gate: DefaultGate()})
	mustPut(t, s, spec{key: "a-rule", body: "Rules never expire."}.req())
	c.advance(3 * 24 * time.Hour)
	got, err := s.Expire(ctx)
	if err != nil || len(got) != 1 || got[0].Key != "short-state" {
		t.Fatalf("expire after 3 days = %v, %v", got, err)
	}
	if v, err := s.Verify(ctx, "pushed-state", ""); err != nil || !v.VerifiedAt.Equal(c.t) || v.Version != 1 {
		t.Fatalf("verify = %+v, %v", v, err)
	}
	c.advance(13 * 24 * time.Hour)
	if got, _ := s.Expire(ctx); len(got) != 0 {
		t.Fatalf("verified record expired early: %v", got)
	}
	c.advance(24 * time.Hour)
	got, _ = s.Expire(ctx)
	if len(got) != 1 || got[0].Key != "pushed-state" || got[0].Status != Expired {
		t.Fatalf("expire after ttl = %+v", got)
	}
	if hits, _ := s.Search(ctx, Query{Text: "agent-guard pushed"}); len(hits) != 0 {
		t.Fatalf("expired record still searchable: %v", keys(hits))
	}
	h, _ := s.History(ctx, "pushed-state")
	if len(h) != 1 || !strings.HasPrefix(h[0].Reason, "expired") {
		t.Fatalf("history = %+v", h)
	}
	res := mustPut(t, s, PutRequest{Key: "pushed-state", Update: true, Body: ptr("agent-guard refactor is pushed."), Gate: DefaultGate()})
	got2, _ := s.Get(ctx, "pushed-state")
	if res.Version != 3 || got2.Status != Current || !got2.ValidTo.IsZero() {
		t.Fatalf("revived = %+v", got2)
	}
	if r, _ := s.Get(ctx, "a-rule"); r.Status != Current {
		t.Fatalf("rule expired")
	}
}

func TestVerify(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	mustPut(t, s, spec{key: "fw-state", title: "SMOK BIOS", body: "SMOK runs BIOS 50E.", kind: State, core: true}.req())
	c.advance(14 * 24 * time.Hour)
	if got, _ := s.Expire(ctx); len(got) != 1 {
		t.Fatalf("expire = %v", got)
	}
	c.advance(time.Hour)
	r, err := s.Verify(ctx, "fw-state", "dmidecode shows 50E")
	if err != nil || r.Status != Current || !r.VerifiedAt.Equal(c.t) || !r.ValidTo.IsZero() || r.Version != 3 || !r.AsOf().Equal(c.t) {
		t.Fatalf("verify of expired = %+v, %v", r, err)
	}
	h, _ := s.History(ctx, "fw-state")
	if len(h) != 2 || h[1].Status != Expired || h[1].Reason != "verified: dmidecode shows 50E" {
		t.Fatalf("history = %+v", h)
	}
	if hits, _ := s.Search(ctx, Query{Text: "SMOK BIOS"}); len(hits) != 1 {
		t.Fatalf("verified record not searchable: %v", keys(hits))
	}
	c.advance(13 * 24 * time.Hour)
	if got, _ := s.Expire(ctx); len(got) != 0 {
		t.Fatalf("expired 13 days after verify: %v", got)
	}
	mustPut(t, s, PutRequest{Key: "fw-state-2", Kind: State, Scopes: []string{GlobalScope}, Body: ptr("SMOK runs BIOS 50F."), Supersedes: []string{"fw-state"}, Gate: Gate{Skip: true}})
	var se *StatusError
	if _, err := s.Verify(ctx, "fw-state", ""); !errors.As(err, &se) || se.SupersededBy != "fw-state-2" {
		t.Fatalf("verify of superseded: %v", err)
	}
	if _, err := s.Verify(ctx, "missing", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("verify of missing: %v", err)
	}
}

func TestSupersedesDeduplicated(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	mustPut(t, s, spec{key: "a", body: "Old rule."}.req())
	req := spec{key: "b", body: "New rule."}.req()
	req.Supersedes = []string{"a", "a"}
	res := mustPut(t, s, req)
	if !slices.Equal(res.Superseded, []string{"a"}) {
		t.Errorf("superseded = %v", res.Superseded)
	}
	h, _ := s.History(ctx, "a")
	r, _ := s.Get(ctx, "a")
	if len(h) != 1 || r.Version != 2 {
		t.Errorf("history %d rows, version %d", len(h), r.Version)
	}
}

func TestUpsertKeepsSourceDates(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	src := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	recs := []Record{
		{Key: "project_old", Kind: State, Scopes: []string{"project:p"}, Body: "old state", ValidFrom: src, UpdatedAt: src},
		{Key: "reference_future", Kind: Fact, Scopes: []string{"project:p"}, Body: "clock skew", ValidFrom: c.t.Add(48 * time.Hour)},
		{Key: "reference_undated", Kind: Fact, Scopes: []string{"project:p"}, Body: "no date"},
	}
	if _, err := s.Upsert(ctx, recs, nil, "legacy"); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Get(ctx, "project_old")
	if !old.AsOf().Equal(src) || !old.ValidFrom.Equal(src) || !old.CreatedAt.Equal(src) {
		t.Errorf("project_old as of %v valid_from %v created %v, want %v", old.AsOf(), old.ValidFrom, old.CreatedAt, src)
	}
	for _, k := range []string{"reference_future", "reference_undated"} {
		if r, _ := s.Get(ctx, k); !r.AsOf().Equal(c.t) || !r.ValidFrom.Equal(c.t) {
			t.Errorf("%s as of %v valid_from %v, want clamped to %v", k, r.AsOf(), r.ValidFrom, c.t)
		}
	}
	if got, _ := s.Expire(ctx); len(got) != 1 || got[0].Key != "project_old" {
		t.Errorf("expire = %v, want project_old past its ttl", got)
	}
	c.advance(time.Hour)
	newer := src.Add(24 * time.Hour)
	recs[0].Body, recs[0].ValidFrom, recs[0].UpdatedAt = "newer state", newer, newer
	if res, err := s.Upsert(ctx, recs[:1], nil, "legacy"); err != nil || len(res.Updated) != 1 {
		t.Fatalf("update = %+v, %v", res, err)
	}
	if r, _ := s.Get(ctx, "project_old"); !r.AsOf().Equal(newer) || !r.ValidFrom.Equal(newer) {
		t.Errorf("updated as of %v valid_from %v, want %v", r.AsOf(), r.ValidFrom, newer)
	}
}

func TestRetire(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	mustPut(t, s, enPush.req())
	if _, err := s.Retire(ctx, enPush.key, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("retire without why: %v", err)
	}
	r, err := s.Retire(ctx, enPush.key, "moved into agent-guard")
	if err != nil || r.Status != Retired || r.ValidTo.IsZero() || r.Version != 2 {
		t.Fatalf("retire = %+v, %v", r, err)
	}
	var se *StatusError
	if _, err := s.Retire(ctx, enPush.key, "again"); !errors.As(err, &se) {
		t.Fatalf("second retire: %v", err)
	}
	if _, err := s.Retire(ctx, "missing", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retire missing: %v", err)
	}
	h, _ := s.History(ctx, enPush.key)
	if len(h) != 1 || h[0].Reason != "retired: moved into agent-guard" {
		t.Fatalf("history = %+v", h)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	seed(t, s)
	if _, err := s.Retire(ctx, "tailscale-buffers", "obsolete"); err != nil {
		t.Fatal(err)
	}
	core := true
	mustPut(t, s, PutRequest{Key: "no-push-default", Update: true, Core: &core})
	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"current", Filter{}, 8},
		{"all", Filter{AllStatus: true}, 9},
		{"retired", Filter{Statuses: []Status{Retired}}, 1},
		{"facts", Filter{Kinds: []Kind{Fact}}, 3},
		{"host scope", Filter{Scopes: []string{"host:smok"}}, 1},
		{"core", Filter{Core: &core}, 1},
	}
	for _, c := range cases {
		got, err := s.List(ctx, c.f)
		if err != nil || len(got) != c.want {
			t.Errorf("%s: %d records, %v; want %d", c.name, len(got), err, c.want)
		}
	}
}

func TestTriggers(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	if _, err := ParseTrigger("Bash"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("trigger without regex: %v", err)
	}
	if _, err := ParseTrigger("Bash:("); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad regex: %v", err)
	}
	push, _ := ParseTrigger(`Bash:\bgit\s+push\b`)
	files, _ := ParseTrigger(`Write|Edit:\.claude/projects/.*/memory/`)
	req := enPush.req()
	req.Triggers = &[]Trigger{push}
	mustPut(t, s, req)
	req = spec{key: "memory-writes", body: "Memory changes go through mem put."}.req()
	req.Triggers = &[]Trigger{files}
	mustPut(t, s, req)
	mustPut(t, s, spec{key: "plain", body: "No triggers."}.req())
	cases := []struct {
		tool, subject string
		scopes        []string
		want          []string
	}{
		{"Bash", "git push origin main", nil, []string{enPush.key}},
		{"Bash", "git push origin main", []string{"host:kurwa"}, nil},
		{"Bash", "git push origin main", []string{"host:kurwa", GlobalScope}, []string{enPush.key}},
		{"Bash", "git status", nil, nil},
		{"Bash", "echo .claude/projects/x/memory/a.md", nil, nil},
		{"Edit", "/home/u/.claude/projects/x/memory/a.md", nil, []string{"memory-writes"}},
		{"Write", "/home/u/.claude/projects/x/memory/a.md", nil, []string{"memory-writes"}},
		{"WriteFile", "/home/u/.claude/projects/x/memory/a.md", nil, nil},
	}
	for _, c := range cases {
		hits, err := s.MatchTriggers(ctx, c.tool, c.subject, c.scopes)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, h := range hits {
			got = append(got, h.Record.Key)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("MatchTriggers(%s, %q) = %v, want %v", c.tool, c.subject, got, c.want)
		}
	}
	all, _ := s.ListByTrigger(ctx, nil)
	if len(all) != 2 {
		t.Errorf("ListByTrigger = %d records", len(all))
	}
}

func TestUpsertIdempotent(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	recs := []Record{
		{Key: "feedback_x", Kind: Rule, Scopes: []string{"project:nixos-config"}, Title: "X", Body: "body x", Source: "legacy:/m/feedback_x.md"},
		{Key: "reference_y#1", Kind: Log, Scopes: []string{"project:nixos-config"}, Body: "chunk 1", Source: "legacy:/m/reference_y.md"},
	}
	links := []Link{{Src: "feedback_x", Dst: "missing_slug", Rel: Unresolved}}
	res, err := s.Upsert(ctx, recs, links, "legacy")
	if err != nil || len(res.Created) != 2 {
		t.Fatalf("first upsert = %+v, %v", res, err)
	}
	c.advance(time.Hour)
	res, err = s.Upsert(ctx, recs, links, "legacy")
	if err != nil || len(res.Created) != 0 || len(res.Updated) != 0 || res.Unchanged != 2 {
		t.Fatalf("second upsert = %+v, %v", res, err)
	}
	if _, err := s.Retire(ctx, "feedback_x", "distilled"); err != nil {
		t.Fatal(err)
	}
	recs[0].Body = "body x changed"
	res, err = s.Upsert(ctx, recs, nil, "legacy")
	if err != nil || !slices.Equal(res.Updated, []string{"feedback_x"}) {
		t.Fatalf("changed upsert = %+v, %v", res, err)
	}
	got, _ := s.Get(ctx, "feedback_x")
	if got.Status != Retired || got.Body != "body x changed" || got.Version != 3 {
		t.Fatalf("upsert must keep status: %+v", got)
	}
	l, _ := s.Links(ctx, "feedback_x")
	if len(l) != 1 {
		t.Fatalf("links = %+v", l)
	}
}

func TestValidation(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	cases := []struct {
		name string
		req  PutRequest
	}{
		{"bad key", spec{key: "Bad Key", body: "x"}.req()},
		{"leading dash", spec{key: "-x", body: "x"}.req()},
		{"bad kind", PutRequest{Key: "k", Kind: "note", Scopes: []string{GlobalScope}, Body: ptr("x")}},
		{"no scope", PutRequest{Key: "k", Kind: Rule, Body: ptr("x")}},
		{"bad scope", PutRequest{Key: "k", Kind: Rule, Scopes: []string{"team:x"}, Body: ptr("x")}},
		{"empty body", PutRequest{Key: "k", Kind: Rule, Scopes: []string{GlobalScope}, Body: ptr("  \n")}},
		{"ttl on rule", PutRequest{Key: "k", Kind: Rule, Scopes: []string{GlobalScope}, Body: ptr("x"), TTLDays: ptr(3)}},
		{"update with supersedes", PutRequest{Key: "k", Update: true, Supersedes: []string{"x"}}},
		{"self supersede", PutRequest{Key: "k", Kind: Rule, Scopes: []string{GlobalScope}, Body: ptr("x"), Supersedes: []string{"k"}}},
	}
	for _, c := range cases {
		if _, err := s.Put(ctx, c.req); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", c.name, err)
		}
	}
	for _, sc := range []string{"global", "project:nixos-config", "host:SMOK", "project:FreeToken"} {
		if _, err := NormalizeScope(sc); err != nil {
			t.Errorf("NormalizeScope(%q): %v", sc, err)
		}
	}
	if HostScope("SMOK.angora-ide.ts.net") != "host:smok" || ProjectScope("FreeToken") != "project:freetoken" {
		t.Errorf("scope helpers")
	}
}

func TestReopenAndReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "d", "mem.db")
	if _, err := Open(ctx, path, Options{ReadOnly: true}); !IsNotExist(err) {
		t.Fatalf("read-only open of missing db: %v", err)
	}
	s, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustPut(t, s, enPush.req())
	_ = s.Close()
	ro, err := Open(ctx, path, Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ro.Close() }()
	if v, _ := ro.SchemaVersion(ctx); v != len(migrations) {
		t.Fatalf("schema version %d", v)
	}
	if _, err := ro.Get(ctx, enPush.key); err != nil {
		t.Fatal(err)
	}
	if _, err := ro.Put(ctx, spec{key: "other", body: "x"}.req()); err == nil {
		t.Fatal("write through read-only store succeeded")
	}
}

func TestFileModes(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "agent-memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mem.db")
	s, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustPut(t, s, enPush.req())
	check := func(when string) {
		t.Helper()
		for _, p := range []string{path, path + "-wal", path + "-shm"} {
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatalf("%s: %v", when, err)
			}
			if fi.Mode().Perm()&0o077 != 0 {
				t.Errorf("%s: %s mode %v", when, filepath.Base(p), fi.Mode().Perm())
			}
		}
	}
	check("new database")
	_ = s.Close()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Get(ctx, enPush.key); err != nil {
		t.Fatal(err)
	}
	check("reopened 0644 database")
}

func TestMigrateV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mem.db")
	s, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, "DROP TRIGGER vectors_text_changed; DROP TABLE vectors; DROP TABLE topics; DROP TABLE auto_topics; DROP TABLE links; CREATE TABLE links (src TEXT NOT NULL, dst TEXT NOT NULL, rel TEXT NOT NULL, PRIMARY KEY (src, dst, rel)) WITHOUT ROWID; "+
		"INSERT INTO links VALUES ('a', 'b', 'relates'); PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if _, err := Open(ctx, path, Options{ReadOnly: true}); !errors.Is(err, ErrSchemaOutdated) {
		t.Fatalf("read-only open of a v1 database: %v", err)
	}
	s, err = Open(ctx, path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if v, _ := s.SchemaVersion(ctx); v != len(migrations) {
		t.Fatalf("schema version %d after migration", v)
	}
	links, err := s.Links(ctx, "a")
	if err != nil || !slices.Equal(links, []Link{{Src: "a", Dst: "b", Rel: Relates}}) {
		t.Fatalf("links after migration %+v, %v", links, err)
	}
}

func TestSnippet(t *testing.T) {
	long := strings.Repeat("padding words here ", 30) + "the Ёлка stands in the corner " + strings.Repeat("tail words ", 30)
	cases := []struct {
		name, body, query string
		n                 int
		want              string
		contains          string
	}{
		{name: "short body", body: "a  b\n c", n: 10, want: "a b c"},
		{name: "head when no query", body: long, n: 20, want: "padding words here p..."},
		{name: "window around match", body: long, query: "елка", n: 40, contains: "Ёлка stands"},
	}
	for _, c := range cases {
		got := Snippet(c.body, c.query, c.n)
		if c.want != "" && got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
		if c.contains != "" && (!strings.Contains(got, c.contains) || !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...")) {
			t.Errorf("%s: %q lacks %q", c.name, got, c.contains)
		}
	}
}

func TestExpiresAt(t *testing.T) {
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		rec  Record
		want time.Time
	}{
		{name: "ttl from update", rec: Record{TTLDays: 3, UpdatedAt: base}, want: base.AddDate(0, 0, 3)},
		{name: "zero ttl is the default", rec: Record{UpdatedAt: base}, want: base.AddDate(0, 0, DefaultStateTTL)},
		{name: "verify moves it", rec: Record{TTLDays: 2, UpdatedAt: base, VerifiedAt: base.AddDate(0, 0, 5)}, want: base.AddDate(0, 0, 7)},
	}
	for _, c := range cases {
		if got := c.rec.ExpiresAt(); !got.Equal(c.want) {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

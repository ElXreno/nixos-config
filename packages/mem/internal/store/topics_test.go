package store

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

func topicSeed(t *testing.T, s *Store, c *clock) {
	t.Helper()
	mustPut(t, s, spec{key: "r-core", title: "Core rule", body: "alpha rule about pushes", core: true}.req())
	c.advance(time.Hour)
	mustPut(t, s, spec{key: "r-plain", title: "Plain rule", body: "bravo rule about builds"}.req())
	c.advance(time.Hour)
	mustPut(t, s, spec{key: "f-new", title: "New fact", body: "charlie fact about disks", kind: Fact}.req())
	c.advance(time.Hour)
	mustPut(t, s, spec{key: "f-old", title: "Old fact", body: "delta fact about fans", kind: Fact, scopes: []string{"project:p"}}.req())
	mustPut(t, s, spec{key: "log#1", body: "echo journal chunk", kind: Log}.req())
	mustPut(t, s, spec{key: "gone", title: "Gone", body: "foxtrot fact about sockets", kind: Fact}.req())
}

func TestSetTopicsAssignsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	topicSeed(t, s, c)
	as := []TopicAssign{
		{"r-core", "git", "Git and pushes"}, {"r-plain", "git", "Git and pushes"},
		{"f-new", "hw", "Hardware"}, {"f-old", "hw", "Hardware"}, {"log#1", "hw", "Hardware"},
		{"missing", "hw", "Hardware"},
	}
	res, err := s.SetTopics(ctx, as)
	if err != nil {
		t.Fatal(err)
	}
	if res.Topics != 2 || res.Titles != 2 || res.Assigned != 5 || res.Unchanged != 0 || !slices.Equal(res.Unknown, []string{"missing"}) {
		t.Fatalf("first run %+v", res)
	}
	res, err = s.SetTopics(ctx, as)
	if err != nil {
		t.Fatal(err)
	}
	if res.Titles != 0 || res.Assigned != 0 || res.Unchanged != 5 {
		t.Fatalf("second run %+v", res)
	}
	got, err := s.Get(ctx, "f-old")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Scopes, []string{"project:p", "topic:hw"}) || got.Topic() != "hw" || got.Version != 1 {
		t.Fatalf("f-old %+v", got)
	}
	topics, err := s.Topics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []TopicInfo{{"git", "Git and pushes", 2, 0}, {"hw", "Hardware", 2, 1}}
	if !slices.Equal(topics, want) {
		t.Fatalf("topics %+v, want %+v", topics, want)
	}
}

func TestSetTopicsMovesAndRetitles(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	topicSeed(t, s, c)
	if _, err := s.SetTopics(ctx, []TopicAssign{{"r-core", "git", "Git"}}); err != nil {
		t.Fatal(err)
	}
	res, err := s.SetTopics(ctx, []TopicAssign{{"r-core", "build", "Builds"}, {"f-new", "git", "Git and pushes"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Assigned != 2 || res.Titles != 2 {
		t.Fatalf("%+v", res)
	}
	got, _ := s.Get(ctx, "r-core")
	if got.Topic() != "build" || len(got.Scopes) != 2 {
		t.Fatalf("scopes %v", got.Scopes)
	}
	topics, _ := s.Topics(ctx)
	want := []TopicInfo{{"build", "Builds", 1, 0}, {"git", "Git and pushes", 1, 0}}
	if !slices.Equal(topics, want) {
		t.Fatalf("topics %+v", topics)
	}
}

func TestSetTopicsRejectsBadInput(t *testing.T) {
	s, c := newStore(t)
	topicSeed(t, s, c)
	cases := []struct {
		name string
		in   []TopicAssign
	}{
		{"two titles", []TopicAssign{{"r-core", "git", "A"}, {"r-plain", "git", "B"}}},
		{"two topics", []TopicAssign{{"r-core", "git", "A"}, {"r-core", "hw", "B"}}},
		{"bad id", []TopicAssign{{"r-core", "Bad Id", "A"}}},
		{"empty title", []TopicAssign{{"r-core", "git", " "}}},
		{"bad key", []TopicAssign{{"R Core", "git", "A"}}},
		{"long title", []TopicAssign{{"r-core", "git", strings.Repeat("x", MaxTitleRunes+1)}}},
	}
	for _, tc := range cases {
		if _, err := s.SetTopics(context.Background(), tc.in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	topics, _ := s.Topics(context.Background())
	if len(topics) != 0 {
		t.Errorf("a rejected map left %+v", topics)
	}
}

func TestOnlyCurrentRecordsCount(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	mustPut(t, s, spec{key: "a", body: "alpha rule", kind: Fact}.req())
	mustPut(t, s, spec{key: "b", body: "bravo statement", kind: Fact}.req())
	if _, err := s.SetTopics(ctx, []TopicAssign{{"a", "t", "T"}, {"b", "t", "T"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retire(ctx, "b", "gone"); err != nil {
		t.Fatal(err)
	}
	topics, _ := s.Topics(ctx)
	if len(topics) != 1 || topics[0].Records != 1 {
		t.Fatalf("%+v", topics)
	}
}

func TestTopicScopeRules(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	topicSeed(t, s, c)
	if _, err := NormalizeScopes([]string{"topic:a", "topic:b"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("two topic scopes: %v", err)
	}
	if got, err := NormalizeScopes([]string{"Topic:Git", "global"}); err != nil || !slices.Equal(got, []string{"global", "topic:git"}) {
		t.Errorf("%v %v", got, err)
	}
	if _, err := s.SetTopics(ctx, []TopicAssign{{"r-plain", "git", "Git"}}); err != nil {
		t.Fatal(err)
	}
	req := PutRequest{Key: "r-plain", Update: true, Scopes: []string{"global", "host:smok"}, Body: ptr("bravo rule about builds, revised")}
	mustPut(t, s, req)
	got, _ := s.Get(ctx, "r-plain")
	if !slices.Equal(got.Scopes, []string{"global", "host:smok", "topic:git"}) {
		t.Errorf("update dropped the topic: %v", got.Scopes)
	}
	req.Scopes = []string{"global", "topic:hw"}
	req.Body = ptr("bravo rule about builds, revised again")
	mustPut(t, s, req)
	got, _ = s.Get(ctx, "r-plain")
	if got.Topic() != "hw" || len(got.Scopes) != 2 {
		t.Errorf("explicit topic lost: %v", got.Scopes)
	}
	res, err := s.Upsert(ctx, []Record{{Key: "r-plain", Kind: Rule, Scopes: []string{"global", "topic:hw"}, Body: "bravo rule about builds, revised again", Title: got.Title}}, nil, "again")
	if err != nil || res.Unchanged != 1 {
		t.Errorf("upsert %+v %v", res, err)
	}
	res, err = s.Upsert(ctx, []Record{{Key: "r-plain", Kind: Rule, Scopes: []string{"global"}, Body: "bravo rule about builds, revised again", Title: got.Title}}, nil, "no topic")
	if err != nil || res.Unchanged != 1 {
		t.Errorf("upsert without a topic scope %+v %v", res, err)
	}
	got, _ = s.Get(ctx, "r-plain")
	if got.Topic() != "hw" {
		t.Errorf("import dropped the topic: %v", got.Scopes)
	}
}

func titled(key, title string) PutRequest {
	req := spec{key: key, title: title, body: "body of " + key + " with its own words " + strings.Repeat(key+" ", 3)}.req()
	req.Gate = Gate{Skip: true}
	return req
}

func keywordSeed(t *testing.T, s *Store, c *clock) {
	t.Helper()
	for _, r := range [][2]string{
		{"g1", "SMOK agent-guard push rule"},
		{"g2", "agent-guard push confirmation"},
		{"g3", "Push after each commit"},
		{"g4", "git push to the fork of nixpkgs"},
		{"h1", "smok fan curve"},
		{"h2", "SMOK NVMe firmware 2026-11-07"},
		{"h3", "SMOK fan bearing noise"},
		{"z1", "ZFS ARC on SMOK"},
		{"z2", "ZFS readahead on 2.4 and on llama.cpp"},
		{"z3", "ZFS gone topic word"},
		{"z4", "ZFS superseded oldword"},
	} {
		mustPut(t, s, titled(r[0], r[1]))
		c.advance(time.Minute)
	}
	mustPut(t, s, titled("z5", "ZFS newer replacement"))
	mustPut(t, s, PutRequest{Key: "z6", Kind: Rule, Scopes: []string{GlobalScope}, Title: ptr("ZFS newer replacement two"), Body: ptr("the second replacement body"),
		Supersedes: []string{"z4"}, Gate: Gate{Skip: true}})
	mustPut(t, s, PutRequest{Key: "z7", Kind: State, Scopes: []string{GlobalScope}, Title: ptr("ZFS stalestate"), Body: ptr("a state that runs out"), TTLDays: ptr(1), Gate: Gate{Skip: true}})
	mustPut(t, s, PutRequest{Key: "z8#1", Kind: Log, Scopes: []string{GlobalScope}, Title: ptr("ZFS logword journal"), Body: ptr("journal chunk"), Gate: Gate{Skip: true}})
	if _, err := s.Retire(context.Background(), "z3", "gone"); err != nil {
		t.Fatal(err)
	}
	var as []TopicAssign
	for _, k := range []string{"g1", "g2", "g3", "g4"} {
		as = append(as, TopicAssign{k, "git", "Git habits"})
	}
	for _, k := range []string{"h1", "h2", "h3"} {
		as = append(as, TopicAssign{k, "smok-hw", "Desktop hardware"})
	}
	for _, k := range []string{"z1", "z2", "z3", "z4", "z5", "z6", "z7", "z8#1"} {
		as = append(as, TopicAssign{k, "storage", "ZFS tuning"})
	}
	if _, err := s.SetTopics(context.Background(), as); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * 24 * time.Hour)
}

func TestTopicMapKeywords(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	keywordSeed(t, s, c)
	m, err := s.TopicMap(ctx, MapKeywords)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TopicEntry{}
	for _, e := range m {
		got[e.ID] = e
	}
	if len(m) != 3 || m[0].ID != "git" || m[0].Records != 4 || got["storage"].Records != 4 || got["smok-hw"].Records != 3 {
		t.Fatalf("%+v", m)
	}
	want := map[string][]string{
		"git":     {"push", "agent-guard", "commit", "confirmation", "fork"},
		"smok-hw": {"fan", "bearing", "curve", "firmware", "noise"},
		"storage": {"newer", "replacement", "ARC", "llama.cpp", "readahead"},
	}
	for id, w := range want {
		if !slices.Equal(got[id].Keywords, w) {
			t.Errorf("%s keywords %q, want %q", id, got[id].Keywords, w)
		}
	}
	for _, e := range m {
		for _, k := range e.Keywords {
			if strings.EqualFold(k, "smok") && e.ID != "git" {
				t.Errorf("%s: %q survives though it names the topic", e.ID, k)
			}
			for _, bad := range []string{"gone", "oldword", "stalestate", "logword", "2026-11-07", "v2.4", "the", "to", "of", "on", "each", "after"} {
				if strings.EqualFold(k, bad) {
					t.Errorf("%s: keyword %q", e.ID, k)
				}
			}
		}
	}
	again, _ := s.TopicMap(ctx, MapKeywords)
	if !slices.EqualFunc(m, again, func(a, b TopicEntry) bool { return a.ID == b.ID && slices.Equal(a.Keywords, b.Keywords) }) {
		t.Error("two runs disagree")
	}
	two, _ := s.TopicMap(ctx, 2)
	for _, e := range two {
		if len(e.Keywords) != 2 || !slices.Equal(e.Keywords, got[e.ID].Keywords[:2]) {
			t.Errorf("%s with 2 keywords: %q", e.ID, e.Keywords)
		}
	}
}

func TestTopicMapKeepsTheCommonCasing(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	for i, title := range []string{"BIMBA cache node", "bimba cache disk", "BIMBA ncps port", "Bimba Tailscale route"} {
		mustPut(t, s, titled(fmt.Sprintf("b%d", i), title))
	}
	mustPut(t, s, titled("o1", "kopia snapshot policy"))
	if _, err := s.SetTopics(ctx, []TopicAssign{{"b0", "net", "Network"}, {"b1", "net", "Network"}, {"b2", "net", "Network"}, {"b3", "net", "Network"}, {"o1", "bak", "Backups"}}); err != nil {
		t.Fatal(err)
	}
	m, err := s.TopicMap(ctx, MapKeywords)
	if err != nil {
		t.Fatal(err)
	}
	if m[0].ID != "net" || !slices.Equal(m[0].Keywords, []string{"BIMBA", "cache", "disk", "ncps", "node"}) {
		t.Fatalf("%+v", m[0])
	}
}

func entries(n int) []TopicEntry {
	out := make([]TopicEntry, n)
	for i := range out {
		out[i] = TopicEntry{ID: "topic-" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Title: "Topic number title " + string(rune('a'+i%26)), Records: 100 - i,
			Keywords: []string{"alpha", "bravo", "charlie", "delta", "echo"}}
	}
	return out
}

func keywordCounts(t *testing.T, out string) []int {
	t.Helper()
	var n []int
	for _, l := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		if !strings.HasPrefix(l, "- ") {
			continue
		}
		_, kws, ok := strings.Cut(l, ": ")
		if !ok {
			n = append(n, 0)
			continue
		}
		n = append(n, len(strings.Split(kws, ", ")))
	}
	return n
}

func TestRenderTopicMapDegradesInsideTheBudget(t *testing.T) {
	es := entries(49)
	for _, budget := range []int{4000, 2500, 1500, 1000, 700, 400, 250} {
		out := RenderTopicMap(es, budget)
		if out == "" {
			if budget >= 400 {
				t.Errorf("budget %d: empty map", budget)
			}
			continue
		}
		if n := runes(out); n > budget {
			t.Errorf("budget %d: %d chars", budget, n)
		}
		if !strings.HasSuffix(out, "\n") || !strings.Contains(out, "mem topics list") || !strings.Contains(out, "--scope topic:ID") {
			t.Errorf("budget %d: %q", budget, out)
		}
	}
	full := RenderTopicMap(es[:3], 2500)
	for _, w := range []string{"- Topic number title a [topic-aa] 100: alpha, bravo, charlie, delta, echo\n", "[topic-ca] 98: alpha"} {
		if !strings.Contains(full, w) {
			t.Errorf("small map lacks %q:\n%s", w, full)
		}
	}
	if out := RenderTopicMap(es, 2500); !strings.Contains(out, "topic-aa") || strings.Contains(out, "more") {
		t.Errorf("biggest topic missing or a tail too early:\n%s", out)
	}
	if out := RenderTopicMap(es, 700); !strings.Contains(out, " more") || strings.Contains(out, "alpha") {
		t.Errorf("compact form expected:\n%s", out)
	}
	if RenderTopicMap(es, 0) != "" || RenderTopicMap(nil, 2500) != "" {
		t.Error("map without a budget or topics must be empty")
	}
}

func TestRenderTopicMapTrimsKeywordsFromTheBiggestTopicsDown(t *testing.T) {
	es := entries(4)
	es[3].Keywords = es[3].Keywords[:2]
	full := RenderTopicMap(es, 10000)
	if got := keywordCounts(t, full); !slices.Equal(got, []int{5, 5, 5, 2}) {
		t.Fatalf("full %v", got)
	}
	steps := [][]int{{4, 5, 5, 2}, {4, 4, 5, 2}, {4, 4, 4, 2}, {3, 4, 4, 2}, {3, 3, 3, 2}, {2, 2, 2, 2}, {1, 2, 2, 2}, {1, 1, 1, 1}, {0, 1, 1, 1}, {0, 0, 0, 0}}
	last := runes(full)
	for _, want := range steps {
		var got []int
		var out string
		for budget := last - 1; budget > 0; budget-- {
			out = RenderTopicMap(es, budget)
			if got = keywordCounts(t, out); slices.Equal(got, want) {
				last = runes(out)
				break
			}
			if runes(out) > budget {
				t.Fatalf("budget %d: %d chars", budget, runes(out))
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("never reached %v, last %v:\n%s", want, got, out)
		}
	}
	out := RenderTopicMap(es, last-1)
	if !strings.Contains(out, "more") && !strings.Contains(out, "topic-da 97\n") {
		t.Fatalf("after the keywords the compact tail must follow:\n%s", out)
	}
}

func TestUsageHeaderIsShortAndLeftOutOfTheCoreFile(t *testing.T) {
	if n := runes(UsageHeader); n > 600 {
		t.Errorf("usage header is %d chars", n)
	}
	for _, w := range []string{"mem search", "mem get", "mem put", "--update", "--supersedes", "--ttl", "--verify", "old memory files"} {
		if !strings.Contains(UsageHeader, w) {
			t.Errorf("usage header lacks %q", w)
		}
	}
	want := "# Agent memory: core records\n\nThis file is generated by `mem core` from the store; `mem get KEY --history` shows earlier versions of a record.\n"
	if coreHeader != want {
		t.Errorf("core header %q, want %q", coreHeader, want)
	}
	s, _ := newStore(t)
	mustPut(t, s, spec{key: "core-rule", title: "Core rule", body: "alpha rule about pushes", core: true}.req())
	out, err := s.RenderCore(context.Background(), CoreScopes, CoreBudget)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, want) || strings.Contains(out, "Memory lives in mem") || strings.Contains(out, "old memory files") {
		t.Errorf("rendered core file:\n%s", out)
	}
}

func TestRenderTopicMapKeepsTitlesOfTheBiggestTopicsFirst(t *testing.T) {
	es := entries(49)
	out := RenderTopicMap(es, 1500)
	if n := runes(out); n > 1500 {
		t.Fatalf("%d chars", n)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	titled := 0
	for _, l := range lines[1 : len(lines)-1] {
		if !strings.HasPrefix(l, "- Topic number title ") || strings.Contains(l, ":") {
			t.Errorf("unexpected titled line %q", l)
		}
		titled++
	}
	last := lines[len(lines)-1]
	if titled == 0 || titled >= 49 || strings.Contains(last, "more") || !strings.Contains(last, es[48].ID+" ") {
		t.Errorf("%d titled lines, compact tail %q", titled, last)
	}
	for i, l := range lines[1 : len(lines)-1] {
		if !strings.Contains(l, "["+es[i].ID+"]") {
			t.Errorf("titled topics must be the biggest first: line %d %q", i, l)
		}
	}
}

func TestTopicMapSkipsStateRecordsPastTheirTTLBeforeExpire(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	mustPut(t, s, PutRequest{Key: "short-state", Kind: State, Scopes: []string{GlobalScope}, Title: ptr("Shortlived state"), Body: ptr("short"), TTLDays: ptr(3), Gate: DefaultGate()})
	mustPut(t, s, spec{key: "keeper", title: "Keeper rule", body: "alpha rule about pushes"}.req())
	if _, err := s.SetTopics(ctx, []TopicAssign{{"short-state", "t", "T"}, {"keeper", "t", "T"}}); err != nil {
		t.Fatal(err)
	}
	c.advance(10 * 24 * time.Hour)
	m, err := s.TopicMap(ctx, MapKeywords)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[0].Records != 1 || !slices.Equal(m[0].Keywords, []string{"Keeper", "rule"}) {
		t.Fatalf("%+v", m)
	}
}

func TestRenderTopicMapNeverExceedsTheBudget(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for range 3000 {
		n := rng.IntN(60)
		es := make([]TopicEntry, n)
		for i := range es {
			e := TopicEntry{ID: fmt.Sprintf("topic-%d", rng.IntN(1000)), Records: 1 + rng.IntN(300)}
			if rng.IntN(5) > 0 {
				e.Title = strings.Repeat("ж", 1+rng.IntN(60))
			}
			for range rng.IntN(MapKeywords + 2) {
				e.Keywords = append(e.Keywords, fmt.Sprintf("kw%d%s", rng.IntN(1000), strings.Repeat("ж", rng.IntN(20))))
			}
			es[i] = e
		}
		budget := rng.IntN(4000)
		out := RenderTopicMap(es, budget)
		if runes(out) > budget {
			t.Fatalf("%d topics, budget %d, got %d chars", n, budget, runes(out))
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			t.Fatalf("no final newline: %q", out)
		}
	}
}

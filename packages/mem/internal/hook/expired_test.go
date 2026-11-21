package hook

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/recall"
	"mem/internal/store"
)

const (
	relayBody    = "The relay board firmware is at revision 7 and the watchdog is disabled."
	relayPointer = "- `relay-state` [state, EXPIRED since 2026-11-05]: Relay board firmware state (unverified: check it, then `mem verify` or `mem retire`)"
)

func (sb *sandbox) seedExpired(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(ctx, []store.Record{{Key: "relay-state", Kind: store.State, Scopes: []string{"global"}, TTLDays: 3,
		Title: "Relay board firmware state", Body: relayBody}}, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	expired, err := s.Expire(ctx)
	if err != nil || len(expired) != 2 {
		t.Fatalf("expire: %v, %d records", err, len(expired))
	}
}

func expiredSandbox(t *testing.T, cos float32) *sandbox {
	t.Helper()
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedExpired(t)
	emb := tierEmbedder()
	emb.cos["relay board firmware"] = cos
	sb.indexPinned(t, emb)
	useEmbedder(t, emb)
	return sb
}

func checkOnlyPointer(t *testing.T, got string) {
	t.Helper()
	strong, ptrs, found := strings.Cut(got, recall.PointerHeader)
	if !found || !strings.Contains(ptrs, relayPointer) {
		t.Errorf("no EXPIRED pointer line:\n%s", got)
	}
	if strings.Contains(strong, "relay-state") || strings.Contains(got, "revision 7") {
		t.Errorf("an expired record was loaded as text:\n%s", got)
	}
}

func TestExpiredStateIsOnlyAPointerOnTheCosineGate(t *testing.T) {
	sb := expiredSandbox(t, 0.90)
	got := sb.prompt(t, "e1", tierFlags()...)
	checkOnlyPointer(t, got)
	if !strings.Contains(got, "`no-push-default` [rule, as of 2026-10-01]: No git push by default. Do not git push") {
		t.Errorf("the current strong record is gone:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "relay-state=0.900") {
		t.Errorf("log line %q", line)
	}
	if again := sb.prompt(t, "e1", tierFlags()...); strings.Contains(again, "relay-state") {
		t.Errorf("the pointer is repeated within a session:\n%s", again)
	}

	below := expiredSandbox(t, justBelow(pointerCos))
	if got := below.prompt(t, "e2", tierFlags()...); strings.Contains(got, "relay-state") {
		t.Errorf("an expired record below the pointer cosine is listed:\n%s", got)
	}

	off := expiredSandbox(t, 0.90)
	if got := off.prompt(t, "e3", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", f32(strongCos)); strings.Contains(got, "relay-state") {
		t.Errorf("an expired record without a pointer tier:\n%s", got)
	}
}

func TestExpiredStateIsOnlyAPointerOnTheRerankGate(t *testing.T) {
	sb := expiredSandbox(t, 0.90)
	scores := deciding()
	scores["relay board firmware"] = 0.9999
	f := &fakeReranker{scores: scores}
	useReranker(t, f)
	got := sb.prompt(t, "x1", rerankFlags()...)
	checkOnlyPointer(t, got)
	strong, _, _ := strings.Cut(got, recall.PointerHeader)
	for _, w := range []string{"`smok-apex-d2` [rule", "`collusion-wiki` [rule"} {
		if !strings.Contains(strong, w) {
			t.Errorf("strong tier lacks %s:\n%s", w, got)
		}
	}
	_, docs, _ := f.seen()
	joined := strings.Join(docs, "|")
	if n := len(docs); n != 8 || !strings.Contains(joined, relayBody) || strings.Contains(joined, "c00c712d") {
		t.Errorf("the reranker saw %d documents, want the 7 current ones and relay-state, not agent-guard-push-state below every pool cosine", n)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "n 8") || !strings.Contains(line, "relay-state=0.9999") {
		t.Errorf("log line %q", line)
	}

	low := expiredSandbox(t, 0.90)
	scores = deciding()
	scores["relay board firmware"] = justBelowPointer
	useReranker(t, &fakeReranker{scores: scores})
	if got := low.prompt(t, "x2", rerankFlags()...); strings.Contains(got, "relay-state") {
		t.Errorf("an expired record below the rerank pointer score is listed:\n%s", got)
	}
}

func TestExpiredStateIsOnlyAPointerOnTheLexicalPath(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedExpired(t)
	stdin := promptInput(t, sb, "l1", "Which relay board firmware revision runs now, and is the watchdog disabled?")
	got := sb.context(t, stdin, UserPromptSubmit)
	checkOnlyPointer(t, got)
	if line := sb.lastLog(t); !strings.Contains(line, "pointers relay-state=lex-expired") {
		t.Errorf("log line %q", line)
	}
	if again := sb.context(t, stdin, UserPromptSubmit); strings.Contains(again, "relay-state") {
		t.Errorf("the pointer is repeated within a session:\n%s", again)
	}

	failing := newSandbox(t)
	failing.seed(t)
	failing.seedExpired(t)
	failing.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{err: context.DeadlineExceeded})
	got = failing.context(t, promptInput(t, failing, "l2", "Which relay board firmware revision runs now, and is the watchdog disabled?"), append([]string{UserPromptSubmit}, vecFlags...)...)
	checkOnlyPointer(t, got)
}

func pointerSection(t *testing.T, got string) string {
	t.Helper()
	_, ptrs, found := strings.Cut(got, recall.PointerHeader)
	if !found {
		t.Fatalf("no pointer tier:\n%s", got)
	}
	return ptrs
}

func TestExpiredPointersComeOnTopOfTheCurrentOnes(t *testing.T) {
	sb := expiredSandbox(t, 0.90)
	ptrs := pointerSection(t, sb.prompt(t, "o1", tierFlags("--pointer-take", "2")...))
	for _, w := range []string{zfsLine, settingsLine, relayPointer} {
		if !strings.Contains(ptrs, w) {
			t.Errorf("cosine gate, pointer take 2: no %q in\n%s", w, ptrs)
		}
	}
	if strings.Index(ptrs, relayPointer) < strings.Index(ptrs, settingsLine) {
		t.Errorf("cosine gate: the expired pointer precedes a current one:\n%s", ptrs)
	}

	rr := expiredSandbox(t, 0.90)
	scores := deciding()
	scores["relay board firmware"] = 0.95
	useReranker(t, &fakeReranker{scores: scores})
	ptrs = pointerSection(t, rr.prompt(t, "o2", rerankFlags("--pointer-take", "2")...))
	for _, w := range []string{"- `no-push-default` [rule", settingsLine, relayPointer} {
		if !strings.Contains(ptrs, w) {
			t.Errorf("rerank gate, pointer take 2: no %q in\n%s", w, ptrs)
		}
	}
	if strings.Index(ptrs, relayPointer) < strings.Index(ptrs, settingsLine) {
		t.Errorf("rerank gate: the expired pointer precedes a current one:\n%s", ptrs)
	}
}

func TestRerankExpiredSeenPointersBlockTheNextOne(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	var recs []store.Record
	for _, k := range []string{"xray", "yankee", "zulu"} {
		recs = append(recs, store.Record{Key: "exp-" + k, Kind: store.State, Scopes: []string{"global"}, TTLDays: 3,
			Title: "Expired " + k + " state", Body: "marker-" + k + " board state body."})
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	emb := tierEmbedder()
	emb.cos["marker-xray"] = 0.85
	emb.cos["marker-yankee"] = 0.80
	emb.cos["marker-zulu"] = 0.75
	sb.indexPinned(t, emb)
	useEmbedder(t, emb)
	f := &fakeReranker{scores: map[string]float64{"marker-xray": 0.99, "marker-yankee": 0.98, "marker-zulu": 0.97}}
	useReranker(t, f)

	first := sb.prompt(t, "seen1", rerankFlags()...)
	if !strings.Contains(first, "exp-xray") || !strings.Contains(first, "exp-yankee") {
		t.Fatalf("turn 1 lacks x and y pointers:\n%s", first)
	}
	second := sb.prompt(t, "seen1", rerankFlags()...)
	_, docs, _ := f.seen()
	joined := strings.Join(docs, "|")
	if !strings.Contains(joined, "marker-zulu") {
		t.Errorf("turn 2: reranker never saw exp-zulu")
	}
	if strings.Contains(joined, "marker-xray") || strings.Contains(joined, "marker-yankee") {
		t.Errorf("turn 2: the reranker was sent pointers already shown")
	}
	if !strings.Contains(second, "exp-zulu") {
		t.Errorf("turn 2: exp-zulu not listed after x and y were shown:\n%s", second)
	}
}

func TestLexicalExpiredHonoursPointerTakeZero(t *testing.T) {
	const q = "Which relay board firmware revision runs now, and is the watchdog disabled?"
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedExpired(t)
	if got := sb.context(t, promptInput(t, sb, "pt0", q), UserPromptSubmit, "--pointer-take", "0"); strings.Contains(got, "relay-state") {
		t.Errorf("lexical pool, --pointer-take 0 lists an expired pointer:\n%s", got)
	}

	failing := newSandbox(t)
	failing.seed(t)
	failing.seedExpired(t)
	failing.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{err: context.DeadlineExceeded})
	args := append(append([]string{UserPromptSubmit}, vecFlags...), "--pointer-take", "0")
	if got := failing.context(t, promptInput(t, failing, "pt1", q), args...); strings.Contains(got, "relay-state") {
		t.Errorf("embedder timeout, --pointer-take 0 lists an expired pointer:\n%s", got)
	}

	ok := newSandbox(t)
	ok.seed(t)
	ok.seedExpired(t)
	ok.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	if got := ok.context(t, promptInput(t, ok, "pt2", q), args...); strings.Contains(got, "relay-state") {
		t.Errorf("healthy embedder, --pointer-take 0 lists an expired pointer:\n%s", got)
	}
}

func TestLexicalExpiredListsAtMostTheExpiredCandidates(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	var recs []store.Record
	for _, k := range []string{"alpha", "bravo", "charlie", "delta"} {
		recs = append(recs, store.Record{Key: "relay-" + k, Kind: store.State, Scopes: []string{"global"}, TTLDays: 3,
			Title: "Relay board firmware " + k, Body: "The relay board firmware watchdog revision " + k + "."})
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	got := sb.context(t, promptInput(t, sb, "cap", "Which relay board firmware watchdog revision runs now?"), UserPromptSubmit)
	if n := strings.Count(got, "EXPIRED since"); n == 0 || n > recall.ExpiredCandidates {
		t.Errorf("%d expired pointers on the lexical path, want 1 to %d:\n%s", n, recall.ExpiredCandidates, got)
	}
}

func TestShadowPointersIncludeExpiredLikeLive(t *testing.T) {
	sb := expiredSandbox(t, 0.90)
	sb.prompt(t, "sx", tierFlags("--shadow-vec-pool", "reg")...)
	lines := sb.shadowLinesPointers(t)
	if len(lines) != 1 {
		t.Fatalf("%d shadow lines", len(lines))
	}
	l := lines[0]
	if !slices.Contains(l.LivePointers, "relay-state") {
		t.Fatalf("live path did not list the expired pointer: %v", l.LivePointers)
	}
	if !slices.Equal(l.LivePointers, l.ShadowPointers) {
		t.Errorf("same pool reg live and shadow, pointers differ: live %v shadow %v", l.LivePointers, l.ShadowPointers)
	}
}

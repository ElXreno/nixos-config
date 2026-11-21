package hook

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"mem/internal/recall"
	"mem/internal/statefile"
)

type fakeReranker struct {
	scores  map[string]float64
	delay   time.Duration
	hold    time.Duration
	err     error
	mutate  func([]float64) []float64
	panics  bool
	calls   atomic.Int32
	mu      sync.Mutex
	query   string
	docs    []string
	timeout time.Duration
}

func (f *fakeReranker) Rerank(ctx context.Context, query string, docs []string) ([]float64, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.query, f.docs = query, slices.Clone(docs)
	f.mu.Unlock()
	if f.panics {
		var empty []float64
		return []float64{empty[3]}, nil
	}
	if f.hold > 0 {
		time.Sleep(f.hold)
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	out := make([]float64, len(docs))
	for i, d := range docs {
		for marker, s := range f.scores {
			if strings.Contains(d, marker) {
				out[i] = s
			}
		}
	}
	if f.mutate != nil {
		out = f.mutate(out)
	}
	return out, nil
}

func (f *fakeReranker) seen() (string, []string, time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.query, slices.Clone(f.docs), f.timeout
}

func useReranker(t *testing.T, f *fakeReranker) {
	t.Helper()
	old := rerankerFor
	rerankerFor = func(_ string, timeout time.Duration) Reranker {
		f.mu.Lock()
		f.timeout = timeout
		f.mu.Unlock()
		return f
	}
	t.Cleanup(func() { rerankerFor = old })
}

var (
	justBelowText    = math.Nextafter(recall.DefaultRerankText, 0)
	justBelowPointer = math.Nextafter(recall.DefaultRerankPointer, 0)
)

func deciding() map[string]float64 {
	return map[string]float64{
		"hangs the embedded controller": 0.999,
		"logs visitors":                 recall.DefaultRerankText,
		"explicit authorization":        justBelowText,
		"installed by home-manager":     recall.DefaultRerankPointer,
		"grows to half of RAM":          justBelowPointer,
		"bricks the Flydigi":            0.1,
	}
}

func rerankFlags(extra ...string) []string {
	return append(tierFlags("--rerank-url", "http://reranker.invalid"), extra...)
}

func TestRerankDecidesBothTiers(t *testing.T) {
	sb := tierSandbox(t)
	f := &fakeReranker{scores: deciding()}
	useReranker(t, f)
	got := sb.prompt(t, "r1", rerankFlags()...)
	strong, ptrs, _ := strings.Cut(got, recall.PointerHeader)
	for _, w := range []string{"`smok-apex-d2` [rule", "`collusion-wiki` [rule"} {
		if !strings.Contains(strong, w) {
			t.Errorf("strong tier lacks %s:\n%s", w, got)
		}
	}
	if strings.Index(strong, "smok-apex-d2") > strings.Index(strong, "collusion-wiki") {
		t.Errorf("strong records are not in rerank order:\n%s", got)
	}
	for _, w := range []string{"`no-push-default` [rule", "`settings-json-copy` [fact"} {
		if !strings.Contains(ptrs, w) {
			t.Errorf("pointer tier lacks %s:\n%s", w, got)
		}
	}
	for _, w := range []string{"bs3pro-0xdf", "zfs-arc-ram"} {
		if strings.Contains(got, w) {
			t.Errorf("output has %s:\n%s", w, got)
		}
	}
	if f.calls.Load() != 1 {
		t.Errorf("%d rerank calls", f.calls.Load())
	}
	line := sb.lastLog(t)
	for _, w := range []string{"embed=ok", "rerank=ok", "n 8", "keys smok-apex-d2=0.000,collusion-wiki=", "rerank smok-apex-d2=0.9990,collusion-wiki=0.9900,no-push-default=0.9900,settings-json-copy=0.9000"} {
		if !strings.Contains(line, w) {
			t.Errorf("log line lacks %q: %s", w, line)
		}
	}
	if c := sb.counters(t); c["ok"] != 1 || c["rerank-ok"] != 1 || len(c) != 2 {
		t.Errorf("counters %v", c)
	}
}

func TestRerankSendsTitleAndClippedBody(t *testing.T) {
	sb := tierSandbox(t)
	f := &fakeReranker{scores: deciding()}
	useReranker(t, f)
	sb.prompt(t, "d1", rerankFlags("--rerank-doc-chars", "12", "--rerank-candidates", "3")...)
	q, docs, _ := f.seen()
	if q != pointerPrompt {
		t.Errorf("query %q", q)
	}
	want := []string{"No git push by default\nDo not git p", "Command 0xDF is never sent to the BS3 Pro\nCommand 0xDF", "ZFS ARC size after boot\nThe ZFS ARC "}
	if !slices.Equal(docs, want) {
		t.Errorf("documents %q, want the top 3 by cosine as title, newline, 12 chars of body", docs)
	}
	sb.prompt(t, "d2", rerankFlags("--rerank-query-chars", "6")...)
	if q, docs, _ := f.seen(); q != "Quokka" || len(docs) != 8 {
		t.Errorf("query %q, %d documents", q, len(docs))
	}
	sb.prompt(t, "d3", rerankFlags()...)
	if _, docs, _ := f.seen(); len(docs) != 8 || !strings.HasPrefix(docs[0], "No git push by default\nDo not git push by default. After a commit, wait for explicit authorization; every push is its own action.") {
		t.Errorf("documents %q", docs)
	}
}

type rerankFailure struct {
	name    string
	f       *fakeReranker
	args    []string
	outcome string
}

func rerankFailures() []rerankFailure {
	return []rerankFailure{
		{"slow", &fakeReranker{scores: deciding(), delay: 3 * time.Second}, []string{"--rerank-timeout", "80ms"}, "timeout"},
		{"ignores its context", &fakeReranker{scores: deciding(), hold: 3 * time.Second}, []string{"--rerank-timeout", "100ms"}, "timeout"},
		{"down", &fakeReranker{err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}, nil, "down"},
		{"garbage error", &fakeReranker{err: errors.New("not a rerank list")}, nil, "response"},
		{"NaN", &fakeReranker{scores: deciding(), mutate: func(s []float64) []float64 { s[0] = math.NaN(); return s }}, nil, "response"},
		{"infinity", &fakeReranker{scores: deciding(), mutate: func(s []float64) []float64 { s[1] = math.Inf(1); return s }}, nil, "response"},
		{"above one", &fakeReranker{scores: deciding(), mutate: func(s []float64) []float64 { s[2] = 1.5; return s }}, nil, "response"},
		{"below zero", &fakeReranker{scores: deciding(), mutate: func(s []float64) []float64 { s[2] = -0.5; return s }}, nil, "response"},
		{"too few scores", &fakeReranker{scores: deciding(), mutate: func(s []float64) []float64 { return s[1:] }}, nil, "response"},
		{"too many scores", &fakeReranker{scores: deciding(), mutate: func(s []float64) []float64 { return append(s, 1) }}, nil, "response"},
		{"nil scores", &fakeReranker{scores: deciding(), mutate: func([]float64) []float64 { return nil }}, nil, "response"},
		{"panic", &fakeReranker{panics: true}, nil, "error"},
	}
}

func cosineReference(t *testing.T, session string, args ...string) string {
	t.Helper()
	ref := tierSandbox(t)
	_, want, _ := ref.run(promptInput(t, ref, session, pointerPrompt), append([]string{UserPromptSubmit}, tierFlags(args...)...)...)
	if want == "" {
		t.Fatal("the cosine reference run injected nothing")
	}
	return want
}

func TestRerankFailuresKeepTheCosineGate(t *testing.T) {
	want := cosineReference(t, "f")
	for _, c := range rerankFailures() {
		t.Run(c.name, func(t *testing.T) {
			sb := tierSandbox(t)
			useReranker(t, c.f)
			start := time.Now()
			code, got, errs := sb.run(promptInput(t, sb, "f", pointerPrompt), append([]string{UserPromptSubmit}, rerankFlags(c.args...)...)...)
			if code != 0 || errs != "" {
				t.Errorf("exit %d stderr %q", code, errs)
			}
			if got != want {
				t.Errorf("output differs from the cosine gate:\n%s\nvs\n%s", got, want)
			}
			if d := time.Since(start); d > 1500*time.Millisecond {
				t.Errorf("took %v", d)
			}
			if cnt := sb.counters(t); cnt["ok"] != 1 || cnt["rerank-"+c.outcome] != 1 || len(cnt) != 2 {
				t.Errorf("counters %v, want ok=1 rerank-%s=1", cnt, c.outcome)
			}
			line := sb.lastLog(t)
			if !strings.Contains(line, "rerank="+c.outcome) || !strings.Contains(line, "inject") || strings.Contains(line, "rerank no-push") {
				t.Errorf("log line %q", line)
			}
			turns := sb.turns(t)
			if len(turns) != 1 || turns[0].Rerank != c.outcome || len(turns[0].Reranked) != 0 {
				t.Fatalf("turns %+v", turns)
			}
			for _, k := range append(append([]statefile.Key{}, turns[0].Strong...), turns[0].Pointers...) {
				if k.Rerank != nil {
					t.Errorf("fallback key %s carries a rerank score", k.Key)
				}
			}
		})
	}
}

func TestRerankOverHTTPFailuresKeepTheCosineGate(t *testing.T) {
	want := cosineReference(t, "h")
	json200 := func(body string) http.HandlerFunc { return rawReply(200, "application/json", body) }
	cases := []struct {
		name    string
		handler http.HandlerFunc
		outcome string
	}{
		{"html page", rawReply(200, "text/html", "<html>captive portal</html>"), "response"},
		{"index out of range", json200(`{"results":[{"index":99,"relevance_score":0.5}]}`), "response"},
		{"wrong count", json200(`{"results":[{"index":0,"relevance_score":0.5}]}`), "response"},
		{"NaN literal", json200(`{"results":[{"index":0,"relevance_score":NaN}]}`), "response"},
		{"http 500", rawReply(500, "application/json", `{"error":{"message":"input is too large to process"}}`), "http"},
		{"never answers", hijackThen(func(net.Conn, *bufio.ReadWriter) { time.Sleep(2 * time.Second) }), "timeout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			sb := tierSandbox(t)
			code, got, _ := sb.run(promptInput(t, sb, "h", pointerPrompt), append([]string{UserPromptSubmit}, tierFlags("--rerank-url", srv.URL, "--rerank-timeout", "300ms")...)...)
			if code != 0 || got != want {
				t.Errorf("exit %d, output differs from the cosine gate:\n%s\nvs\n%s", code, got, want)
			}
			if cnt := sb.counters(t); cnt["rerank-"+c.outcome] != 1 {
				t.Errorf("counters %v", cnt)
			}
		})
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down := "http://" + ln.Addr().String()
	_ = ln.Close()
	sb := tierSandbox(t)
	code, got, _ := sb.run(promptInput(t, sb, "h", pointerPrompt), append([]string{UserPromptSubmit}, tierFlags("--rerank-url", down)...)...)
	if code != 0 || got != want || sb.counters(t)["rerank-down"] != 1 {
		t.Errorf("refused port: exit %d counters %v output\n%s", code, sb.counters(t), got)
	}
}

func TestRerankOverHTTP(t *testing.T) {
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		var req struct {
			Query     string   `json:"query"`
			Documents []string `json:"documents"`
			TopN      int      `json:"top_n"`
		}
		if r.URL.Path != "/v1/rerank" || json.NewDecoder(r.Body).Decode(&req) != nil || req.TopN != len(req.Documents) {
			http.Error(w, "bad", 400)
			return
		}
		type res struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		}
		var out []res
		for i, d := range req.Documents {
			s := 0.0
			for m, v := range deciding() {
				if strings.Contains(d, m) {
					s = v
				}
			}
			out = append(out, res{i, s})
		}
		slices.Reverse(out)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	defer srv.Close()
	sb := tierSandbox(t)
	direct := tierSandbox(t)
	useReranker(t, &fakeReranker{scores: deciding()})
	want := direct.prompt(t, "o1", rerankFlags()...)
	rerankerFor = func(url string, timeout time.Duration) Reranker { return defaultReranker(url, timeout) }
	got := sb.prompt(t, "o1", tierFlags("--rerank-url", srv.URL)...)
	if got != want || reqs.Load() != 1 {
		t.Errorf("%d requests, http output differs from the in-process one:\n%s\nvs\n%s", reqs.Load(), got, want)
	}
}

func TestRerankFlagOffIsByteIdentical(t *testing.T) {
	for _, shadow := range []bool{false, true} {
		var extra []string
		if shadow {
			extra = []string{"--shadow-vec-pool", "fused-reg"}
		}
		a, b := tierSandbox(t), tierSandbox(t)
		f := &fakeReranker{scores: deciding()}
		useReranker(t, f)
		outA := a.prompt(t, "z", tierFlags(extra...)...)
		outB := b.prompt(t, "z", tierFlags(append([]string{"--rerank-url", ""}, extra...)...)...)
		if outA != outB || outA == "" {
			t.Errorf("empty --rerank-url changed the output:\n%s\nvs\n%s", outA, outB)
		}
		lex := tierSandbox(t)
		lex.vars["MEM_RERANK_URL"] = "http://reranker.invalid"
		lex.prompt(t, "z", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--reg-cos", "0.7")
		if f.calls.Load() != 0 {
			t.Errorf("reranker called %d times without a live vector pool", f.calls.Load())
		}
		for _, sb := range []*sandbox{a, b, lex} {
			for _, p := range []string{sb.turnsPath(), sb.logPath(), sb.shadowPath()} {
				data, err := os.ReadFile(p)
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if strings.Contains(string(data), "rerank") {
					t.Errorf("%s mentions rerank with the reranker off: %s", p, data)
				}
			}
		}
		ta, tb := a.turns(t), b.turns(t)
		ta[0].TotalMs, tb[0].TotalMs, ta[0].EmbedMs, tb[0].EmbedMs = 0, 0, 0, 0
		ta[0].Cwd, tb[0].Cwd = "", ""
		ja, _ := json.Marshal(ta)
		jb, _ := json.Marshal(tb)
		if string(ja) != string(jb) {
			t.Errorf("turns differ:\n%s\n%s", ja, jb)
		}
	}
}

func TestRerankEnvironmentURL(t *testing.T) {
	sb := tierSandbox(t)
	f := &fakeReranker{scores: deciding()}
	useReranker(t, f)
	sb.vars["MEM_RERANK_URL"] = "http://reranker.invalid"
	got := sb.prompt(t, "e1", tierFlags()...)
	if f.calls.Load() != 1 || !strings.Contains(got, "smok-apex-d2") {
		t.Errorf("%d calls:\n%s", f.calls.Load(), got)
	}
}

func TestRerankTimeoutIsClampedToTheBudget(t *testing.T) {
	want := cosineReference(t, "b", "--budget", "600ms")
	sb := tierSandbox(t)
	f := &fakeReranker{scores: deciding(), delay: 5 * time.Second}
	useReranker(t, f)
	start := time.Now()
	code, got, _ := sb.run(promptInput(t, sb, "b", pointerPrompt), append([]string{UserPromptSubmit}, rerankFlags("--rerank-timeout", "10s", "--budget", "600ms")...)...)
	took := time.Since(start)
	if code != 0 || got != want {
		t.Errorf("exit %d, output differs from the cosine gate:\n%s\nvs\n%s", code, got, want)
	}
	if took > 900*time.Millisecond {
		t.Errorf("took %v with a budget of 600ms", took)
	}
	if _, _, limit := f.seen(); limit <= 0 || limit > 450*time.Millisecond {
		t.Errorf("reranker timeout %v, want at most 3/4 of the remaining budget", limit)
	}
	if c := sb.counters(t); c["rerank-timeout"] != 1 {
		t.Errorf("counters %v", c)
	}
	sb2 := tierSandbox(t)
	f2 := &fakeReranker{scores: deciding()}
	useReranker(t, f2)
	sb2.prompt(t, "b2", rerankFlags("--budget", "7s")...)
	if _, _, limit := f2.seen(); limit != 1500*time.Millisecond {
		t.Errorf("default rerank timeout %v", limit)
	}
}

func TestRerankSessionDedupAndReset(t *testing.T) {
	sb := tierSandbox(t)
	useReranker(t, &fakeReranker{scores: deciding()})
	first := sb.prompt(t, "s1", rerankFlags()...)
	if first == "" {
		t.Fatal("nothing injected")
	}
	if got := sb.prompt(t, "s1", rerankFlags()...); got != "" {
		t.Errorf("second prompt repeats:\n%s", got)
	}
	start, _ := json.Marshal(map[string]string{"session_id": "s1", "cwd": sb.root + "/repos/nixos-config", "hook_event_name": "SessionStart", "source": "compact"})
	if core := sb.context(t, string(start), SessionStart); !strings.Contains(core, "smok-apex-d2") {
		t.Fatalf("compact did not inject the core record again:\n%s", core)
	}
	got := sb.prompt(t, "s1", rerankFlags()...)
	for _, k := range []string{"collusion-wiki", "no-push-default", "settings-json-copy"} {
		if !strings.Contains(got, "`"+k+"`") {
			t.Errorf("after compact %s is not back:\n%s", k, got)
		}
	}
	if strings.Contains(got, "smok-apex-d2") {
		t.Errorf("the core record from session start is repeated:\n%s", got)
	}
	useReranker(t, &fakeReranker{scores: map[string]float64{"explicit authorization": 0.999, "installed by home-manager": 0.995}})
	sb2 := tierSandbox(t)
	sb2.prompt(t, "s2", rerankFlags()...)
	useReranker(t, &fakeReranker{scores: deciding()})
	got = sb2.prompt(t, "s2", rerankFlags()...)
	if strings.Contains(got, "no-push-default") || strings.Contains(got, "settings-json-copy") {
		t.Errorf("seen records came back:\n%s", got)
	}
	if !strings.Contains(got, "smok-apex-d2") {
		t.Errorf("fresh strong record missing:\n%s", got)
	}
}

func TestRerankPointerSeenIsPromotedButNotRepeated(t *testing.T) {
	sb := tierSandbox(t)
	useReranker(t, &fakeReranker{scores: deciding()})
	sb.prompt(t, "p1", rerankFlags()...)
	useReranker(t, &fakeReranker{scores: map[string]float64{"explicit authorization": 0.999, "installed by home-manager": 0.95, "grows to half of RAM": 0.95}})
	got := sb.prompt(t, "p1", rerankFlags()...)
	if !strings.Contains(got, "`no-push-default` [rule, as of 2026-10-01]: No git push by default. Do not git push") {
		t.Errorf("a pointed record that now passes the text gate is not injected in full:\n%s", got)
	}
	if strings.Contains(got, "settings-json-copy") || !strings.Contains(got, "zfs-arc-ram") {
		t.Errorf("pointer dedup:\n%s", got)
	}
}

func TestRerankTurnsAndShadowCarryScores(t *testing.T) {
	sb := tierSandbox(t)
	useReranker(t, &fakeReranker{scores: deciding()})
	sb.prompt(t, "t1", rerankFlags("--shadow-vec-pool", "reg")...)
	turns := sb.turns(t)
	if len(turns) != 1 {
		t.Fatalf("%d turns", len(turns))
	}
	l := turns[0]
	if l.Rerank != "ok" || l.RerankMs < 0 || len(l.Reranked) != 8 {
		t.Errorf("rerank fields %q %d %+v", l.Rerank, l.RerankMs, l.Reranked)
	}
	if !slices.Equal(keysOf(l.Strong), []string{"smok-apex-d2", "collusion-wiki"}) || !slices.Equal(keysOf(l.Pointers), []string{"no-push-default", "settings-json-copy"}) {
		t.Fatalf("strong %+v pointers %+v", l.Strong, l.Pointers)
	}
	wantCos := map[string]float32{"no-push-default": 0.8, "settings-json-copy": pointerCos, "smok-apex-d2": 0, "collusion-wiki": justBelow(pointerCos)}
	for _, k := range append(append([]statefile.Key{}, l.Strong...), l.Pointers...) {
		if k.Rerank == nil {
			t.Errorf("%s lacks its rerank score", k.Key)
			continue
		}
		if want := roundCos(deciding()[markerOf(k.Key)]); *k.Rerank != want {
			t.Errorf("%s rerank %v, want %v", k.Key, *k.Rerank, want)
		}
		if d := k.Cos - roundCos(float64(wantCos[k.Key])); math.Abs(d) > 1e-9 {
			t.Errorf("%s cos %v", k.Key, k.Cos)
		}
	}
	for _, k := range l.Reranked {
		if k.Rerank == nil {
			t.Errorf("candidate %s lacks a score", k.Key)
		}
	}
	data, _ := os.ReadFile(sb.shadowPath())
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &raw); err != nil {
		t.Fatal(err)
	}
	var live []string
	_ = json.Unmarshal(raw["live_keys"], &live)
	var shadowKeys []string
	_ = json.Unmarshal(raw["shadow_keys"], &shadowKeys)
	var reranked []statefile.Key
	_ = json.Unmarshal(raw["live_reranked"], &reranked)
	if string(raw["live_rerank"]) != `"ok"` || len(reranked) != 8 {
		t.Errorf("shadow rerank fields %s %s", raw["live_rerank"], raw["live_reranked"])
	}
	if !slices.Equal(live, []string{"smok-apex-d2", "collusion-wiki"}) || !slices.Equal(shadowKeys, []string{"no-push-default", "bs3pro-0xdf"}) {
		t.Errorf("live %v shadow %v: the shadow keeps the cosine gate", live, shadowKeys)
	}
	if strings.Contains(string(data), "Quokka") {
		t.Error("shadow carries prompt text")
	}
}

func markerOf(key string) string {
	return map[string]string{"smok-apex-d2": "hangs the embedded controller", "collusion-wiki": "logs visitors", "no-push-default": "explicit authorization", "settings-json-copy": "installed by home-manager"}[key]
}

func TestRerankErrorTextStaysOutOfTheLogs(t *testing.T) {
	sb := tierSandbox(t)
	useReranker(t, &fakeReranker{err: errors.New("cannot rank Quokka wombat ping")})
	sb.prompt(t, "x1", rerankFlags("--shadow-vec-pool", "reg")...)
	for _, p := range []string{sb.logPath(), sb.shadowPath()} {
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), "Quokka") {
			t.Errorf("%s carries prompt text: %s", p, data)
		}
	}
}

func TestRerankBadFlagsKeepTheCosineGate(t *testing.T) {
	want := cosineReference(t, "bf")
	for _, extra := range [][]string{
		{"--rerank-candidates", "0"},
		{"--rerank-timeout", "0s"},
		{"--rerank-text", "1.5"},
		{"--rerank-text", "-0.1"},
		{"--rerank-pointer", "0.995"},
		{"--rerank-pointer", "-0.1"},
		{"--rerank-take", "-1"},
		{"--rerank-doc-chars", "-1"},
		{"--rerank-query-chars", "-1"},
	} {
		t.Run(strings.Join(extra, " "), func(t *testing.T) {
			sb := tierSandbox(t)
			f := &fakeReranker{scores: deciding()}
			useReranker(t, f)
			code, got, _ := sb.run(promptInput(t, sb, "bf", pointerPrompt), append([]string{UserPromptSubmit}, rerankFlags(extra...)...)...)
			if code != 0 || got != want || f.calls.Load() != 0 {
				t.Errorf("exit %d, %d calls, output\n%s", code, f.calls.Load(), got)
			}
			data, _ := os.ReadFile(sb.logPath())
			if !strings.Contains(string(data), "rerank off: ") {
				t.Errorf("log %s", data)
			}
		})
	}
}

func TestRerankOnlyPointersAndTakeZero(t *testing.T) {
	sb := tierSandbox(t)
	useReranker(t, &fakeReranker{scores: deciding()})
	got := sb.prompt(t, "o1", rerankFlags("--rerank-take", "0")...)
	if strings.Contains(got, recall.PromptHeader) || !strings.HasPrefix(got, recall.PointerHeader) {
		t.Errorf("take 0 leaves pointers only:\n%s", got)
	}
	for _, k := range []string{"smok-apex-d2", "collusion-wiki", "no-push-default"} {
		if !strings.Contains(got, "`"+k+"`") {
			t.Errorf("pointer %s missing:\n%s", k, got)
		}
	}
	got = sb.prompt(t, "o2", rerankFlags("--rerank-pointer", "0")...)
	if strings.Contains(got, recall.PointerHeader) || !strings.Contains(got, "smok-apex-d2") {
		t.Errorf("pointer 0:\n%s", got)
	}
}

func TestRerankNothingRelevantInjectsNothing(t *testing.T) {
	sb := tierSandbox(t)
	useReranker(t, &fakeReranker{scores: map[string]float64{"explicit authorization": 0.13}})
	if got := sb.prompt(t, "n1", rerankFlags()...); got != "" {
		t.Errorf("an irrelevant prompt got:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "none") || !strings.Contains(line, "rerank=ok") {
		t.Errorf("log line %q", line)
	}
}

func TestSessionStartPrewarmsTheReranker(t *testing.T) {
	listen := func() (string, chan struct{}) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		conns := make(chan struct{}, 4)
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				conns <- struct{}{}
				buf := make([]byte, 1)
				_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				if n, _ := c.Read(buf); n > 0 {
					t.Errorf("the prewarm sent data")
				}
				_ = c.Close()
			}
		}()
		return "http://" + ln.Addr().String(), conns
	}
	for _, viaEnv := range []bool{false, true} {
		embedURL, embedConns := listen()
		rerankURL, rerankConns := listen()
		sb := newSandbox(t)
		sb.seed(t)
		var args []string
		if viaEnv {
			sb.vars["MEM_EMBED_URL"], sb.vars["MEM_RERANK_URL"] = embedURL, rerankURL
		} else {
			args = []string{"--embed-url", embedURL, "--rerank-url", rerankURL}
		}
		sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "rp"), args: args,
			has: []string{"smok-apex-d2"}, log: []string{"inject", "prewarm=ok", "prewarm-rerank=ok"}})
		for name, ch := range map[string]chan struct{}{"embedder": embedConns, "reranker": rerankConns} {
			select {
			case <-ch:
			case <-time.After(time.Second):
				t.Errorf("env %v: no connection reached the %s", viaEnv, name)
			}
		}
	}
}

func TestSessionStartRerankPrewarmIsBoundedAndOptional(t *testing.T) {
	old := prewarmControl
	prewarmControl = func(ctx context.Context, _, _ string, _ syscall.RawConn) error {
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { prewarmControl = old })
	sb := newSandbox(t)
	sb.seed(t)
	sb.vars["MEM_EMBED_URL"], sb.vars["MEM_RERANK_URL"] = "http://192.0.2.1:8080", "http://192.0.2.1:8101"
	start := time.Now()
	sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "rb"),
		has: []string{"smok-apex-d2"}, log: []string{"prewarm=timeout", "prewarm-rerank=timeout"}})
	if d := time.Since(start); d > 600*time.Millisecond {
		t.Errorf("took %v: the two dials do not run side by side", d)
	}
	sb2 := newSandbox(t)
	sb2.seed(t)
	sb2.vars["MEM_RERANK_URL"] = "::not a url::"
	sb2.check(t, spec{name: "bad rerank url", event: SessionStart, stdin: sessionStartInput(sb2, "rc"), has: []string{"smok-apex-d2"}, log: []string{"prewarm-rerank=skip"}})
	if line := sb2.lastLog(t); strings.Contains(line, "prewarm=") {
		t.Errorf("embed prewarm without an embed url: %s", line)
	}
}

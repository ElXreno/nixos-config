package hook

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mem/internal/counter"
	"mem/internal/recall"
	"mem/internal/store"
)

const fakeModel = "fake-embed"

func keywordVec(text string) []float32 {
	lower := strings.ToLower(text)
	v := make([]float32, 5)
	v[4] = 0.1
	for axis, words := range [][]string{{"push", "commits", "upstream"}, {"arc", "zfs", "memory pressure"}, {"0xdf", "cooler"}, {"bluetooth"}} {
		for _, w := range words {
			if strings.Contains(lower, w) {
				v[axis] = 1
				break
			}
		}
	}
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / math.Sqrt(s))
	}
	return v
}

type keywordEmbedder struct {
	delay  time.Duration
	err    error
	mutate func([]float32) []float32
	calls  atomic.Int32
	last   atomic.Value
}

func (k *keywordEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	k.calls.Add(1)
	k.last.Store(texts[0])
	if k.delay > 0 {
		select {
		case <-time.After(k.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if k.err != nil {
		return nil, k.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = keywordVec(t)
		if k.mutate != nil {
			out[i] = k.mutate(out[i])
		}
	}
	return out, nil
}

func useEmbedder(t *testing.T, e recall.Embedder) {
	t.Helper()
	old := embedderFor
	embedderFor = func(string, string, time.Duration) recall.Embedder { return e }
	t.Cleanup(func() { embedderFor = old })
}

func (sb *sandbox) indexVectors(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := recall.Reindex(ctx, s, recall.ReindexOptions{Model: fakeModel, DocPrefix: "Document: ", Embedder: &keywordEmbedder{}, Batch: 4, Par: 1}); err != nil {
		t.Fatal(err)
	}
}

func (sb *sandbox) counters(t *testing.T) map[string]int64 {
	t.Helper()
	got, err := counter.Counts(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "counters"))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

var vecFlags = []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", "0.6"}

func TestVectorPathFindsAParaphrase(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	emb := &keywordEmbedder{}
	useEmbedder(t, emb)
	paraphrase := promptInput(t, sb, "v1", "Should I send my commits upstream already?")
	sb.check(t, spec{name: "lexical default finds nothing", event: UserPromptSubmit, stdin: paraphrase, empty: true, log: []string{"session=v1 none"}})
	if emb.calls.Load() != 0 {
		t.Errorf("the lexical default called the embedder %d times", emb.calls.Load())
	}
	sb.check(t, spec{name: "vector pool finds it", event: UserPromptSubmit, stdin: promptInput(t, sb, "v2", "Should I send my commits upstream already?"),
		args: vecFlags, has: []string{"`no-push-default` [rule"}, lacks: []string{"push-rule-old", "bt-journal"}, log: []string{"inject", "embed=ok"}})
	if got, _ := emb.last.Load().(string); got != "Query: Should I send my commits upstream already?" {
		t.Errorf("embedded text %q", got)
	}
	sb.check(t, spec{name: "session dedup applies to vector hits", event: UserPromptSubmit, stdin: promptInput(t, sb, "v2", "Should I send my commits upstream already?"),
		args: vecFlags, empty: true, log: []string{"session=v2 none"}})
	if got := sb.counters(t); got["ok"] != 2 {
		t.Errorf("counters %v", got)
	}
}

func TestVectorFallbacks(t *testing.T) {
	prompt := func(sb *sandbox, session string) string {
		return promptInput(t, sb, session, "Can I git push the commit to the remote now, or do I wait for your authorization first?")
	}
	cases := []struct {
		name    string
		emb     recall.Embedder
		args    []string
		counter string
		maxMs   int64
	}{
		{"embedder down", &keywordEmbedder{err: errors.New("dial tcp: connection refused")}, nil, "response", 1500},
		{"slow embedder", &keywordEmbedder{delay: 3 * time.Second}, []string{"--embed-timeout", "80ms"}, "timeout", 1500},
		{"wrong dimension", &keywordEmbedder{mutate: func(v []float32) []float32 { return v[:3] }}, nil, "dim", 1500},
		{"NaN vector", &keywordEmbedder{mutate: func(v []float32) []float32 { v[0] = float32(math.NaN()); return v }}, nil, "response", 1500},
		{"infinite vector", &keywordEmbedder{mutate: func(v []float32) []float32 { v[1] = float32(math.Inf(1)); return v }}, nil, "response", 1500},
		{"zero vector", &keywordEmbedder{mutate: func(v []float32) []float32 { return make([]float32, len(v)) }}, nil, "response", 1500},
		{"no vectors for the model", &keywordEmbedder{}, []string{"--embed-model", "unindexed"}, "novectors", 1500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.indexVectors(t)
			useEmbedder(t, c.emb)
			base := prompt(sb, "lex")
			_, want, _ := sb.run(base, UserPromptSubmit)
			if want == "" {
				t.Fatal("the lexical reference run injected nothing")
			}
			sb2 := newSandbox(t)
			sb2.seed(t)
			sb2.indexVectors(t)
			start := time.Now()
			code, got, errs := sb2.run(prompt(sb2, "lex"), append([]string{UserPromptSubmit}, append(append([]string{}, vecFlags...), c.args...)...)...)
			if code != 0 || errs != "" {
				t.Errorf("exit %d, stderr %q", code, errs)
			}
			if withoutNotice(t, got) != want || !strings.Contains(got, "lexical only (embedder "+c.counter+")") {
				t.Errorf("fallback output differs from the lexical path plus the %s notice:\n%s\nvs\n%s", c.counter, got, want)
			}
			if d := time.Since(start).Milliseconds(); d > c.maxMs {
				t.Errorf("took %d ms", d)
			}
			counts := sb2.counters(t)
			if counts[c.counter] != 1 || len(counts) != 1 {
				t.Errorf("counters %v, want %s=1", counts, c.counter)
			}
			if line := sb2.lastLog(t); !strings.Contains(line, "embed="+c.counter) || !strings.Contains(line, "inject") {
				t.Errorf("log line %q", line)
			}
		})
	}
}

func TestVectorFallbackNeverBlocksOnNoMatchingPrompt(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{delay: 5 * time.Second})
	start := time.Now()
	args := append(append([]string{}, vecFlags...), "--embed-timeout", "100ms")
	sb.check(t, spec{name: "slow embedder, nothing lexical", event: UserPromptSubmit, stdin: promptInput(t, sb, "n1", "Explain quantum chromodynamics lattice simulation parameters in detail, with references."),
		args: args, has: []string{"lexical only (embedder timeout)"}, lacks: []string{"`" + "no-push-default"}, log: []string{"none", "embed=timeout"}})
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v", d)
	}
}

func TestVectorEmbedRunsWhileLexicalSearchRuns(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{delay: 150 * time.Millisecond})
	start := time.Now()
	sb.check(t, spec{name: "delayed embedder", event: UserPromptSubmit, stdin: promptInput(t, sb, "d1", "Should I send my commits upstream already?"),
		args: vecFlags, has: []string{"no-push-default"}})
	if d := time.Since(start); d < 150*time.Millisecond || d > 1500*time.Millisecond {
		t.Errorf("took %v", d)
	}
}

func TestVectorLexRegularAndLogs(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	logArgs := []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "all", "--vec-lex-regular", "--reg-take", "0", "--log-take", "1", "--log-cos", "0.6", "--min-cover", "0.1", "--min-shared", "2"}
	sb.check(t, spec{name: "lexical regular plus a vector log", event: UserPromptSubmit,
		stdin: promptInput(t, sb, "l1", "Can I git push the commit to the remote now? The bluetooth headset pairing failed again on the tablet."),
		args:  logArgs, has: []string{"`no-push-default` [rule", "`bt-journal#1` [log"}, log: []string{"embed=ok"}})
	sb.check(t, spec{name: "logs stay out without the log gate", event: UserPromptSubmit,
		stdin: promptInput(t, sb, "l2", "The bluetooth headset pairing failed again on the tablet."),
		args:  []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "all", "--vec-lex-regular"}, empty: true})
	sb.check(t, spec{name: "reg pool never injects logs", event: UserPromptSubmit,
		stdin: promptInput(t, sb, "l3", "The bluetooth headset pairing failed again on the tablet."),
		args:  append(append([]string{}, vecFlags...), "--log-take", "1", "--log-cos", "0.1"), empty: true})
}

func TestVectorFusedPools(t *testing.T) {
	for _, pool := range []string{"fused-reg", "fused-all"} {
		t.Run(pool, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.indexVectors(t)
			useEmbedder(t, &keywordEmbedder{})
			sb.check(t, spec{name: pool, event: UserPromptSubmit, stdin: promptInput(t, sb, "f1", "Should I send my commits upstream already?"),
				args: []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", pool, "--reg-take", "3", "--reg-cos", "0.6"},
				has:  []string{"`no-push-default` [rule"}, log: []string{"embed=ok"}})
		})
	}
}

func TestVectorOverHTTP(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model != fakeModel {
			http.Error(w, "bad request", 400)
			return
		}
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		for i, in := range req.Input {
			data = append(data, item{i, keywordVec(in)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	args := append([]string{}, vecFlags...)
	args[1] = srv.URL
	sb.check(t, spec{name: "http embedder", event: UserPromptSubmit, stdin: promptInput(t, sb, "h1", "Should I send my commits upstream already?"),
		args: args, has: []string{"`no-push-default` [rule"}, log: []string{"embed=ok"}})
	if hits.Load() != 1 {
		t.Errorf("%d embedding requests", hits.Load())
	}
	srv.Close()
	sb.check(t, spec{name: "server gone", event: UserPromptSubmit, stdin: promptInput(t, sb, "h2", "Can I git push the commit to the remote now, or do I wait for your authorization first?"),
		args: args, has: []string{"no-push-default"}, log: []string{"embed=down"}})
}

func TestVectorFlagsAreChecked(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	in := promptInput(t, sb, "x1", "Can I git push the commit to the remote now, or do I wait for your authorization first?")
	cases := [][]string{
		{"--vec-pool", "nonsense", "--embed-url", "http://x", "--embed-model", "m"},
		{"--vec-pool", "reg"},
		{"--vec-pool", "reg", "--embed-url", "http://x"},
		{"--embed-timeout", "0s", "--vec-pool", "reg", "--embed-url", "http://x", "--embed-model", "m"},
	}
	for _, args := range cases {
		code, out, _ := sb.run(in, append([]string{UserPromptSubmit}, args...)...)
		if code != 0 || out != "" {
			t.Errorf("%v: exit %d stdout %q", args, code, out)
		}
		if line := sb.lastLog(t); !strings.Contains(line, "bad flags") {
			t.Errorf("%v: log %q", args, line)
		}
	}
}

func TestEnvDefaultsForEmbedFlags(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	sb.vars["MEM_EMBED_URL"] = "http://embedder.invalid"
	sb.vars["MEM_EMBED_MODEL"] = fakeModel
	sb.check(t, spec{name: "environment supplies url and model", event: UserPromptSubmit, stdin: promptInput(t, sb, "e1", "Should I send my commits upstream already?"),
		args: []string{"--vec-pool", "reg", "--reg-cos", "0.6"}, has: []string{"no-push-default"}})
}

func TestHookMigratesAnOutdatedDatabase(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, "DROP TRIGGER vectors_text_changed; DROP TABLE vectors; DROP TABLE topics; DROP TABLE auto_topics; PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	sb.check(t, spec{name: "recall on a v2 database", event: UserPromptSubmit, fixture: "prompt-push", has: []string{"no-push-default"}})
	s, err = store.Open(ctx, sb.dbPath(), store.Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("database still outdated: %v", err)
	}
	_ = s.Close()
	if _, err := os.Stat(sb.logPath()); err != nil {
		t.Error(err)
	}
}

func TestFreshRecordIsFoundLexicallyBeforeReindex(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	body := "The wireguard-go tunnel on BIMBA needs a 16 MiB socket buffer, otherwise the UDP receive queue drops packets under load."
	if _, err := s.Put(ctx, store.PutRequest{Key: "wg-socket-buffer", Kind: store.Fact, Scopes: []string{"global"}, Title: ptr("WireGuard socket buffer"), Body: &body, Gate: store.DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	prompt := "Why does the wireguard-go tunnel on BIMBA drop UDP packets under load, what socket buffer does it need?"
	sb.check(t, spec{name: "lexical fallback for a record without a vector", event: UserPromptSubmit, stdin: promptInput(t, sb, "fr1", prompt),
		args: vecFlags, has: []string{"`wg-socket-buffer` [fact"}, log: []string{"embed=ok", "lexical gate for 1 records without a vector", "wg-socket-buffer=lex-novec"}})
	sb.check(t, spec{name: "lexical path alone", event: UserPromptSubmit, stdin: promptInput(t, sb, "fr2", prompt),
		has: []string{"`wg-socket-buffer` [fact"}})
}
func ptr[T any](v T) *T { return &v }

package hook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"mem/internal/recall"
)

const shadowPrompt = "Should I send my commits upstream already?"

func (sb *sandbox) shadowPath() string {
	return filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "shadow.jsonl")
}

type shadowHitLine struct {
	Key  string  `json:"key"`
	Cos  float64 `json:"cos"`
	Log  bool    `json:"log"`
	Gate string  `json:"gate"`
}

type shadowLineT struct {
	Time        string   `json:"time"`
	Session     string   `json:"session"`
	Agent       string   `json:"agent"`
	Prompt      string   `json:"prompt_sha"`
	PromptChars int      `json:"prompt_chars"`
	Pool        string   `json:"pool"`
	LiveMode    string   `json:"live_mode"`
	LiveKeys    []string `json:"live_keys"`
	Hits        []shadowHitLine
	ShadowKeys  []string `json:"shadow_keys"`
	Embed       string   `json:"embed"`
	EmbedMs     int64    `json:"embed_ms"`
	Error       string   `json:"error"`
}

func (sb *sandbox) shadowLines(t *testing.T) []shadowLineT {
	t.Helper()
	data, err := os.ReadFile(sb.shadowPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []shadowLineT
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var s shadowLineT
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(l), &raw); err != nil {
			t.Fatalf("shadow line is not JSON: %v: %s", err, l)
		}
		if err := json.Unmarshal([]byte(l), &s); err != nil {
			t.Fatal(err)
		}
		if h, ok := raw["shadow_hits"]; ok {
			if err := json.Unmarshal(h, &s.Hits); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, s)
	}
	return out
}

type embedderCase struct {
	name     string
	emb      recall.Embedder
	args     []string
	embed    string
	maxTotal time.Duration
}

func embedderCases() []embedderCase {
	return []embedderCase{
		{"working", &keywordEmbedder{}, nil, "ok", 1500 * time.Millisecond},
		{"slow", &keywordEmbedder{delay: 3 * time.Second}, []string{"--embed-timeout", "80ms"}, "timeout", 1500 * time.Millisecond},
		{"down", &keywordEmbedder{err: errors.New("dial tcp: connection refused")}, nil, "response", 1500 * time.Millisecond},
		{"NaN", &keywordEmbedder{mutate: func(v []float32) []float32 { v[0] = float32(math.NaN()); return v }}, nil, "response", 1500 * time.Millisecond},
		{"wrong dimension", &keywordEmbedder{mutate: func(v []float32) []float32 { return v[:3] }}, nil, "dim", 1500 * time.Millisecond},
		{"zero vector", &keywordEmbedder{mutate: func(v []float32) []float32 { return make([]float32, len(v)) }}, nil, "response", 1500 * time.Millisecond},
		{"panic", panicEmbedder{}, nil, "error", 1500 * time.Millisecond},
		{"ignores its context", stubbornEmbedder{hold: 4 * time.Second}, []string{"--embed-timeout", "100ms"}, "timeout", 1500 * time.Millisecond},
	}
}

var shadowBase = []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--reg-cos", "0.6"}

func TestShadowNeverChangesTheLiveOutput(t *testing.T) {
	prompts := []string{shadowPrompt, "Can I git push the commit to the remote now, or do I wait for your authorization first?", "Explain quantum chromodynamics arc lattice simulation parameters in detail."}
	for _, live := range []string{"lexical", "reg", "fused-all"} {
		for _, c := range embedderCases() {
			t.Run(live+"/"+c.name, func(t *testing.T) {
				useEmbedder(t, c.emb)
				liveArgs := append(append([]string{}, shadowBase...), c.args...)
				if live != "lexical" {
					liveArgs = append(liveArgs, "--vec-pool", live)
				}
				a, b := newSandbox(t), newSandbox(t)
				for _, sb := range []*sandbox{a, b} {
					sb.seed(t)
					sb.indexVectors(t)
				}
				for i, p := range prompts {
					id := "s" + string(rune('a'+i))
					codeA, outA, errA := a.run(promptInput(t, a, id, p), append([]string{UserPromptSubmit}, liveArgs...)...)
					start := time.Now()
					codeB, outB, errB := b.run(promptInput(t, b, id, p), append(append([]string{UserPromptSubmit}, liveArgs...), "--shadow-vec-pool", "fused-reg")...)
					if d := time.Since(start); d > c.maxTotal {
						t.Errorf("prompt %d: took %v with the shadow on", i, d)
					}
					if codeA != codeB || outA != outB || errA != errB {
						t.Errorf("prompt %d: live output changed by the shadow\nwithout: %d %q %q\nwith:    %d %q %q", i, codeA, outA, errA, codeB, outB, errB)
					}
				}
				if _, err := os.Stat(a.shadowPath()); err == nil {
					t.Error("shadow file written without the flag")
				}
				lines := b.shadowLines(t)
				if len(lines) != len(prompts) {
					t.Fatalf("%d shadow lines, want %d", len(lines), len(prompts))
				}
				for i, l := range lines {
					if l.Embed != c.embed || l.Pool != "fused-reg" || l.Session != "s"+string(rune('a'+i)) {
						t.Errorf("line %d: %+v", i, l)
					}
					if c.embed != "ok" && (len(l.Hits) != 0 || len(l.ShadowKeys) != 0) {
						t.Errorf("line %d: hits without an embedding: %+v", i, l)
					}
				}
				sa, _ := os.ReadDir(filepath.Join(a.vars["XDG_STATE_HOME"], "agent-memory", "sessions"))
				sbDir, _ := os.ReadDir(filepath.Join(b.vars["XDG_STATE_HOME"], "agent-memory", "sessions"))
				if len(sa) != len(sbDir) {
					t.Errorf("session state differs: %d files without, %d with", len(sa), len(sbDir))
				}
				for _, f := range sa {
					x, _ := os.ReadFile(filepath.Join(a.vars["XDG_STATE_HOME"], "agent-memory", "sessions", f.Name()))
					y, _ := os.ReadFile(filepath.Join(b.vars["XDG_STATE_HOME"], "agent-memory", "sessions", f.Name()))
					if !bytes.Equal(x, y) {
						t.Errorf("shadow changed the dedup state of %s", f.Name())
					}
				}
			})
		}
	}
}

func TestShadowLineContent(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	args := append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "all")
	in := promptInput(t, sb, "c1", shadowPrompt)
	code, out, _ := sb.run(in, args...)
	if code != 0 || out != "" {
		t.Fatalf("a paraphrase must not be injected by the lexical path: %d %q", code, out)
	}
	lines := sb.shadowLines(t)
	if len(lines) != 1 {
		t.Fatalf("%d lines", len(lines))
	}
	l := lines[0]
	sum := sha256.Sum256([]byte(shadowPrompt))
	if l.Prompt != hex.EncodeToString(sum[:8]) || l.PromptChars != len([]rune(shadowPrompt)) || l.Session != "c1" || l.Pool != "all" || l.LiveMode != "lexical" || len(l.LiveKeys) != 0 {
		t.Errorf("%+v", l)
	}
	if _, err := time.Parse(time.RFC3339, l.Time); err != nil {
		t.Errorf("time %q: %v", l.Time, err)
	}
	if l.Embed != "ok" || !slices.Contains(l.ShadowKeys, "no-push-default") {
		t.Errorf("%+v", l)
	}
	var first *shadowHitLine
	passes := 0
	for i := range l.Hits {
		h := &l.Hits[i]
		if h.Key == "no-push-default" {
			first = h
		}
		if h.Gate == "pass" {
			passes++
		}
	}
	if first == nil || first.Gate != "pass" || first.Cos < 0.6 || first.Log || passes != len(l.ShadowKeys) {
		t.Errorf("hits %+v", l.Hits)
	}
	if len(l.Hits) < 2 {
		t.Errorf("the pool before the gate is not logged: %+v", l.Hits)
	}
	fi, err := os.Stat(sb.shadowPath())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("shadow file %v %v", fi, err)
	}
	data, _ := os.ReadFile(sb.shadowPath())
	for _, w := range []string{"commits", "upstream", "Should I"} {
		if strings.Contains(string(data), w) {
			t.Errorf("shadow file carries prompt text %q", w)
		}
	}

	live := promptInput(t, sb, "c2", "Can I git push the commit to the remote now, or do I wait for your authorization first?")
	_, out, _ = sb.run(live, args...)
	if out == "" {
		t.Fatal("the lexical reference run injected nothing")
	}
	lines = sb.shadowLines(t)
	if len(lines) != 2 || len(lines[1].LiveKeys) == 0 || !slices.Contains(lines[1].LiveKeys, "no-push-default") {
		t.Errorf("live keys %+v", lines)
	}
}

func TestShadowSkipsWhatTheLivePathSkips(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	args := append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "reg")
	for _, p := range []string{"", "   ", "/compact now"} {
		sb.run(promptInput(t, sb, "k1", p), args...)
	}
	if n := len(sb.shadowLines(t)); n != 0 {
		t.Errorf("%d shadow lines for skipped prompts", n)
	}
}

func TestShadowOutputIsNotWaitingForTheEmbedder(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{delay: 400 * time.Millisecond})
	var out, errb bytes.Buffer
	e := sb.env(promptInput(t, sb, "t1", "Can I git push the commit to the remote now, or do I wait for your authorization first?"), &out, &errb)
	tw := &timedWriter{}
	e.Stdout = tw
	start := time.Now()
	args := append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "reg", "--embed-timeout", "1s")
	code := Run(args, e)
	total := time.Since(start)
	if code != 0 {
		t.Fatal(code)
	}
	if tw.first.IsZero() || tw.first.Sub(start) > 200*time.Millisecond {
		t.Errorf("live output written %v after the start", tw.first.Sub(start))
	}
	if total < 400*time.Millisecond {
		t.Errorf("shadow did not wait for the embedder: %v", total)
	}
	if !strings.Contains(tw.String(), "no-push-default") {
		t.Errorf("stdout %q", tw.String())
	}
	if l := sb.shadowLines(t); len(l) != 1 || l[0].Embed != "ok" {
		t.Errorf("%+v", l)
	}
}

type timedWriter struct {
	mu    sync.Mutex
	first time.Time
	buf   bytes.Buffer
}

func (w *timedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.first.IsZero() {
		w.first = time.Now()
	}
	return w.buf.Write(p)
}

func (w *timedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestShadowSharesTheLiveEmbedding(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	emb := &keywordEmbedder{}
	useEmbedder(t, emb)
	sb.run(promptInput(t, sb, "p1", shadowPrompt), append(append([]string{UserPromptSubmit}, shadowBase...), "--vec-pool", "reg", "--shadow-vec-pool", "fused-all")...)
	if n := emb.calls.Load(); n != 1 {
		t.Errorf("%d embedding requests for one prompt", n)
	}
	l := sb.shadowLines(t)
	if len(l) != 1 || l[0].LiveMode != "vector" || !slices.Contains(l[0].LiveKeys, "no-push-default") || !slices.Equal(l[0].LiveKeys, l[0].ShadowKeys) {
		t.Errorf("%+v", l)
	}
}

func TestShadowPoolsAndGateVerdicts(t *testing.T) {
	for _, pool := range []string{"reg", "fused-reg", "all", "fused-all"} {
		t.Run(pool, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.indexVectors(t)
			useEmbedder(t, &keywordEmbedder{})
			sb.run(promptInput(t, sb, "g1", shadowPrompt), append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", pool, "--reg-take", "1", "--reg-margin", "0.9")...)
			l := sb.shadowLines(t)
			if len(l) != 1 || l[0].Pool != pool || l[0].Embed != "ok" {
				t.Fatalf("%+v", l)
			}
			if len(l[0].ShadowKeys) != 0 {
				t.Errorf("a margin of 0.9 cannot pass: %+v", l[0])
			}
			seen := map[string]bool{}
			for _, h := range l[0].Hits {
				seen[h.Gate] = true
				if h.Gate == "pass" {
					t.Errorf("%+v", h)
				}
			}
			if !seen["margin"] {
				t.Errorf("no margin verdict: %+v", l[0].Hits)
			}
		})
	}
}

func TestShadowOverHTTPAndNoDatabase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0,0,0,0]}]}`))
	}))
	defer srv.Close()
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	args := []string{UserPromptSubmit, "--embed-url", srv.URL, "--embed-model", fakeModel, "--shadow-vec-pool", "reg"}
	code, out, errs := sb.run(promptInput(t, sb, "h1", shadowPrompt), args...)
	if code != 0 || out != "" || errs != "" {
		t.Errorf("%d %q %q", code, out, errs)
	}
	if l := sb.shadowLines(t); len(l) != 1 || l[0].Embed != "ok" || len(l[0].Hits) == 0 {
		t.Errorf("%+v", l)
	}
	empty := newSandbox(t)
	code, out, _ = empty.run(promptInput(t, empty, "h2", shadowPrompt), args...)
	if code != 0 || out != "" {
		t.Errorf("%d %q", code, out)
	}
	if _, err := os.Stat(empty.dbPath()); err == nil {
		t.Error("shadow created the database")
	}
}

func TestShadowEnvDefaults(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	sb.vars["MEM_EMBED_URL"] = "http://embedder.invalid"
	sb.vars["MEM_EMBED_MODEL"] = fakeModel
	sb.run(promptInput(t, sb, "e1", shadowPrompt), UserPromptSubmit, "--shadow-vec-pool", "reg", "--reg-cos", "0.6")
	if l := sb.shadowLines(t); len(l) != 1 || l[0].Embed != "ok" || !slices.Contains(l[0].ShadowKeys, "no-push-default") {
		t.Errorf("%+v", l)
	}
}

func TestShadowMisconfigurationKeepsTheLiveOutput(t *testing.T) {
	prompt := "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	for _, live := range [][]string{nil, {"--vec-pool", "reg", "--embed-url", "http://x", "--embed-model", "m"}} {
		ref := newSandbox(t)
		ref.seed(t)
		_, want, _ := ref.run(promptInput(t, ref, "x1", prompt), append([]string{UserPromptSubmit}, live...)...)
		if !strings.Contains(want, "no-push-default") {
			t.Fatalf("reference output %q", want)
		}
		for _, bad := range [][]string{
			{"--shadow-vec-pool", "nonsense"},
			{"--shadow-vec-pool", "reg"},
			{"--shadow-vec-pool", "reg", "--embed-url", "http://x"},
			{"--shadow-vec-pool", "reg", "--embed-url", "http://x", "--embed-model", "m", "--embed-timeout", "0s"},
		} {
			if len(live) > 0 && !slices.Contains(bad, "nonsense") {
				continue
			}
			sb := newSandbox(t)
			sb.seed(t)
			args := append(append([]string{UserPromptSubmit}, live...), bad...)
			code, out, _ := sb.run(promptInput(t, sb, "x1", prompt), args...)
			if code != 0 || out != want {
				t.Errorf("%v: live output changed by a bad shadow flag: %d %q", args, code, out)
			}
			if _, err := os.Stat(sb.shadowPath()); err == nil {
				t.Errorf("%v: shadow file written", args)
			}
			data, _ := os.ReadFile(sb.logPath())
			if !strings.Contains(string(data), "shadow off") {
				t.Errorf("%v: log %q", args, data)
			}
		}
	}
}

func TestShadowBadLiveFlagsStillDisableTheHook(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	code, out, _ := sb.run(promptInput(t, sb, "x1", shadowPrompt), UserPromptSubmit, "--vec-pool", "reg", "--shadow-vec-pool", "reg")
	if code != 0 || out != "" {
		t.Errorf("%d %q", code, out)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "bad flags") {
		t.Errorf("%q", line)
	}
}

func TestShadowReleasesStderrWithStdout(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{delay: 500 * time.Millisecond})
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	e := sb.env(promptInput(t, sb, "t1", "Can I git push the commit to the remote now, or do I wait for your authorization first?"), &out, &errb)
	e.Stderr = wr
	start := time.Now()
	eof := make(chan time.Duration, 1)
	go func() {
		_, _ = io.Copy(io.Discard, rd)
		eof <- time.Since(start)
	}()
	args := append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "reg", "--embed-timeout", "1s")
	Run(args, e)
	select {
	case d := <-eof:
		if d > 300*time.Millisecond {
			t.Errorf("stderr stayed open for %v", d)
		}
	case <-time.After(3 * time.Second):
		t.Error("stderr never closed")
	}
}

func TestShadowLineAfterAPartialLineStaysParseable(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	if err := os.MkdirAll(filepath.Dir(sb.shadowPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sb.shadowPath(), []byte(`{"time":"2026-11-07T00:00:00Z","sess`), 0o600); err != nil {
		t.Fatal(err)
	}
	sb.run(promptInput(t, sb, "p1", shadowPrompt), append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "reg")...)
	data, err := os.ReadFile(sb.shadowPath())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines: %q", len(lines), data)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &v); err != nil || v["session"] != "p1" {
		t.Errorf("new line is not parseable: %v %q", err, lines[1])
	}
}

func TestShadowLogIsRotated(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	if err := os.MkdirAll(filepath.Dir(sb.shadowPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sb.shadowPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(sb.shadowPath(), maxShadowSize+1); err != nil {
		t.Fatal(err)
	}
	sb.run(promptInput(t, sb, "p1", shadowPrompt), append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "reg")...)
	if fi, err := os.Stat(sb.shadowPath() + ".1"); err != nil || fi.Size() <= maxShadowSize {
		t.Errorf("old log not kept: %v", err)
	}
	if l := sb.shadowLines(t); len(l) != 1 {
		t.Errorf("%d lines in the fresh log", len(l))
	}
}

func TestShadowDiskFullNeverFailsTheHook(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full")
	}
	prompt := "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	ref := newSandbox(t)
	ref.seed(t)
	ref.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	args := append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "reg")
	_, want, _ := ref.run(promptInput(t, ref, "d1", prompt), args...)
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	if err := os.MkdirAll(filepath.Dir(sb.shadowPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/full", sb.shadowPath()); err != nil {
		t.Fatal(err)
	}
	code, out, errs := sb.run(promptInput(t, sb, "d1", prompt), args...)
	if code != 0 || out != want || errs != "" {
		t.Errorf("%d %q %q, want %q", code, out, errs, want)
	}
	data, _ := os.ReadFile(sb.logPath())
	if !strings.Contains(string(data), "shadow:") {
		t.Errorf("log %q", data)
	}
}

type echoErrEmbedder struct{}

func (echoErrEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("server rejected input: " + texts[0])
}

func TestShadowAndLogKeepThePromptText(t *testing.T) {
	const marker = "zxqvunique-marker-phrase"
	prompt := "Can I git push " + marker + " now?"
	for _, emb := range []recall.Embedder{echoErrEmbedder{}, &keywordEmbedder{}} {
		sb := newSandbox(t)
		sb.seed(t)
		sb.indexVectors(t)
		useEmbedder(t, emb)
		args := append(append([]string{UserPromptSubmit}, shadowBase...), "--shadow-vec-pool", "fused-all", "--vec-pool", "reg")
		sb.run(promptInput(t, sb, "m1", prompt), args...)
		sb.run(promptInput(t, sb, "m2", prompt), append(args[:len(args):len(args)], "--db", filepath.Join(sb.root, "absent.db"))...)
		for _, p := range []string{sb.shadowPath(), sb.logPath()} {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), marker) || strings.Contains(string(data), "git push") {
				t.Errorf("%s carries prompt text: %s", filepath.Base(p), data)
			}
		}
	}
}

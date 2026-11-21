package vec

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testDim = 8

func fakeVec(s string, dim int) []float64 {
	sum := sha256.Sum256([]byte(s))
	out := make([]float64, dim)
	for i := range out {
		out[i] = float64(int(sum[i])-128) / 64
	}
	return out
}

type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	inputs   []string
	requests atomic.Int32
	fail     func(n int32) (int, string)
	reply    func(inputs []string) any
}

func newFake(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model_id":"fake/model","version":"1.9.4"}`))
	})
	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		n := f.requests.Add(1)
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.mu.Lock()
		f.inputs = append(f.inputs, req.Input...)
		f.mu.Unlock()
		if f.fail != nil {
			if code, body := f.fail(n); code != 0 {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(strings.ReplaceAll(body, "$INPUT", req.Input[0])))
				return
			}
		}
		if f.reply != nil {
			_ = json.NewEncoder(w).Encode(f.reply(req.Input))
			return
		}
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i := len(req.Input) - 1; i >= 0; i-- {
			data = append(data, item{i, fakeVec(req.Input[i], testDim)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func writeExport(t *testing.T, dir string) (docs, queries []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var d, q bytes.Buffer
	for i := range 11 {
		text := strings.Repeat("doc text ", i+1) + fmt.Sprint(i)
		docs = append(docs, text)
		_ = json.NewEncoder(&d).Encode(map[string]string{"key": fmt.Sprintf("key_%02d", i), "text": text})
	}
	for i := range 7 {
		text := fmt.Sprintf("query %d %s", i, strings.Repeat("x", i*5))
		queries = append(queries, text)
		_ = json.NewEncoder(&q).Encode(map[string]string{"id": fmt.Sprintf("s:%d", i), "session": "s", "set": "gold", "text": text})
	}
	for name, b := range map[string]*bytes.Buffer{"docs.jsonl": &d, "queries.jsonl": &q} {
		if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return docs, queries
}

func norm(v []float64) []float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	s = math.Sqrt(s)
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / s
	}
	return out
}

func readF32(t *testing.T, path string) []float32 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]float32, len(data)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4*i:]))
	}
	return out
}

func baseOptions(srv *fakeServer, in, out string) Options {
	return Options{URL: srv.URL, Model: "fake", In: in, Out: out, Which: "both", Batch: 3, Par: 3, Retries: 2, Backoff: time.Millisecond}
}

func TestEmbed(t *testing.T) {
	srv := newFake(t)
	root := t.TempDir()
	in, out := filepath.Join(root, "in"), filepath.Join(root, "out")
	docs, queries := writeExport(t, in)
	o := baseOptions(srv, in, out)
	o.QueryPrefix, o.DocPrefix = "Q: ", "D: "
	o.QuerySuffix = "<eos>"
	meta, err := Embed(t.Context(), o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Dim != testDim || meta.Docs.Count != 11 || meta.Queries.Count != 7 || meta.Model != "fake" || meta.WallSeconds <= 0 || meta.Docs.TextsPerSecond <= 0 {
		t.Fatalf("meta %+v", meta)
	}
	if !strings.Contains(string(meta.ServerInfo), "fake/model") || meta.ServerEndpoint != "/info" {
		t.Errorf("server info %s from %s", meta.ServerInfo, meta.ServerEndpoint)
	}
	for _, name := range []string{"docs.f32", "docs.keys", "queries.f32", "queries.ids", "meta.json"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", name, info, err)
		}
	}
	got := readF32(t, filepath.Join(out, "docs.f32"))
	if len(got) != 11*testDim {
		t.Fatalf("docs.f32 has %d floats", len(got))
	}
	for i, text := range docs {
		want := norm(fakeVec("D: "+text, testDim))
		for j := range testDim {
			if math.Abs(float64(got[i*testDim+j])-want[j]) > 1e-6 {
				t.Fatalf("doc %d component %d: %v vs %v", i, j, got[i*testDim+j], want[j])
			}
		}
	}
	gotQ := readF32(t, filepath.Join(out, "queries.f32"))
	for i, text := range queries {
		want := norm(fakeVec("Q: "+text+"<eos>", testDim))
		for j := range testDim {
			if math.Abs(float64(gotQ[i*testDim+j])-want[j]) > 1e-6 {
				t.Fatalf("query %d component %d", i, j)
			}
		}
	}
	keys, _ := os.ReadFile(filepath.Join(out, "docs.keys"))
	if !strings.HasPrefix(string(keys), "key_00\nkey_01\n") || strings.Count(string(keys), "\n") != 11 {
		t.Errorf("docs.keys %q", keys)
	}
	ids, _ := os.ReadFile(filepath.Join(out, "queries.ids"))
	if !strings.HasPrefix(string(ids), "s:0\ns:1\n") {
		t.Errorf("queries.ids %q", ids)
	}
	raw, _ := os.ReadFile(filepath.Join(in, "docs.jsonl"))
	sum := sha256.Sum256(raw)
	if meta.Docs.InputSHA256 != hex.EncodeToString(sum[:]) || len(meta.Docs.SentSHA256) != 64 || meta.Docs.SentSHA256 == meta.Docs.InputSHA256 {
		t.Errorf("hashes %+v", meta.Docs)
	}
	read, err := ReadMeta(out)
	if err != nil || read.Dim != testDim || read.Docs.Count != 11 || read.QueryPrefix != "Q: " {
		t.Errorf("meta.json %+v %v", read, err)
	}
	set, err := Load(out, PartDocs)
	if err != nil || len(set.Names) != 11 || set.Dim != testDim || len(set.Data) != 11*testDim {
		t.Fatalf("Load: %v", err)
	}
	for i := range set.Names {
		var s float64
		for _, x := range set.Row(i) {
			s += float64(x) * float64(x)
		}
		if math.Abs(s-1) > 1e-5 {
			t.Errorf("row %d norm^2 %v", i, s)
		}
	}
	seenQ, seenD := 0, 0
	for _, s := range srv.inputs {
		if strings.HasPrefix(s, "Q: ") && strings.HasSuffix(s, "<eos>") {
			seenQ++
		}
		if strings.HasPrefix(s, "D: ") && !strings.HasSuffix(s, "<eos>") {
			seenD++
		}
	}
	if seenQ != 7 || seenD != 11 {
		t.Errorf("prefixed inputs: queries %d docs %d", seenQ, seenD)
	}
}

func TestEmbedWhichAndMerge(t *testing.T) {
	srv := newFake(t)
	root := t.TempDir()
	in, out := filepath.Join(root, "in"), filepath.Join(root, "out")
	writeExport(t, in)
	o := baseOptions(srv, in, out)
	o.Which = "docs"
	if _, err := Embed(t.Context(), o, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "queries.f32")); err == nil {
		t.Fatal("--which docs wrote queries")
	}
	o.Which = "queries"
	if _, err := Embed(t.Context(), o, nil); err != nil {
		t.Fatal(err)
	}
	m, err := ReadMeta(out)
	if err != nil || m.Docs == nil || m.Queries == nil || m.Docs.Count != 11 || m.Queries.Count != 7 {
		t.Fatalf("merged meta %+v %v", m, err)
	}
	o.Model = "other"
	if _, err := Embed(t.Context(), o, nil); err == nil || !strings.Contains(err.Error(), "different") {
		t.Errorf("model change over an existing run: %v", err)
	}
	o.Model, o.DocPrefix, o.Which = "fake", "changed: ", "docs"
	if _, err := Embed(t.Context(), o, nil); err == nil || !strings.Contains(err.Error(), "different") {
		t.Errorf("prefix change over an existing run: %v", err)
	}
}

func TestEmbedDocsFile(t *testing.T) {
	srv := newFake(t)
	root := t.TempDir()
	in, out := filepath.Join(root, "in"), filepath.Join(root, "out")
	writeExport(t, in)
	alt := filepath.Join(root, "docs-en.jsonl")
	if err := os.WriteFile(alt, []byte(`{"key":"k1","text":"english one"}`+"\n"+`{"key":"k2","text":"english two"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := baseOptions(srv, in, out)
	o.Which, o.DocsFile = "docs", alt
	meta, err := Embed(t.Context(), o, nil)
	if err != nil || meta.Docs.Count != 2 || meta.Docs.Source != alt {
		t.Fatalf("%+v %v", meta, err)
	}
	keys, _ := os.ReadFile(filepath.Join(out, "docs.keys"))
	if string(keys) != "k1\nk2\n" {
		t.Errorf("keys %q", keys)
	}
}

func TestEmbedRetries(t *testing.T) {
	srv := newFake(t)
	srv.fail = func(n int32) (int, string) {
		if n <= 2 {
			return 503, "busy"
		}
		return 0, ""
	}
	root := t.TempDir()
	in, out := filepath.Join(root, "in"), filepath.Join(root, "out")
	writeExport(t, in)
	o := baseOptions(srv, in, out)
	o.Which, o.Batch, o.Par, o.Retries = "queries", 100, 1, 3
	if _, err := Embed(t.Context(), o, nil); err != nil {
		t.Fatalf("retried request failed: %v", err)
	}
	if n := srv.requests.Load(); n != 3 {
		t.Errorf("%d requests, want 2 failures and 1 success", n)
	}
	srv2 := newFake(t)
	srv2.fail = func(int32) (int, string) { return 503, "busy" }
	o = baseOptions(srv2, in, filepath.Join(root, "out2"))
	o.Which, o.Batch, o.Par, o.Retries = "queries", 100, 1, 2
	if _, err := Embed(t.Context(), o, nil); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("exhausted retries: %v", err)
	}
	if n := srv2.requests.Load(); n != 3 {
		t.Errorf("%d requests, want 1 try + 2 retries", n)
	}
}

func TestEmbedClientErrorNotRetriedAndNoTextLeak(t *testing.T) {
	srv := newFake(t)
	srv.fail = func(int32) (int, string) { return 422, `{"error":"input too long: $INPUT"}` }
	root := t.TempDir()
	in, out := filepath.Join(root, "in"), filepath.Join(root, "out")
	writeExport(t, in)
	o := baseOptions(srv, in, out)
	o.Which, o.Batch, o.Par = "queries", 100, 1
	_, err := Embed(t.Context(), o, nil)
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("422: %v", err)
	}
	if strings.Contains(err.Error(), "query 0") {
		t.Errorf("error leaks the input text: %v", err)
	}
	if n := srv.requests.Load(); n != 1 {
		t.Errorf("422 retried: %d requests", n)
	}
}

func TestEmbedRejectsBadVectors(t *testing.T) {
	type item struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	}
	good := func(in []string) []item {
		var out []item
		for i, s := range in {
			out = append(out, item{i, fakeVec(s, testDim)})
		}
		return out
	}
	cases := []struct {
		name  string
		reply func(in []string) any
		want  string
	}{
		{"dimension mismatch", func(in []string) any {
			d := good(in)
			d[len(d)-1].Embedding = d[len(d)-1].Embedding[:5]
			return map[string]any{"data": d}
		}, "dimension"},
		{"zero vector", func(in []string) any {
			d := good(in)
			d[0].Embedding = make([]float64, testDim)
			return map[string]any{"data": d}
		}, "zero"},
		{"float32 overflow", func(in []string) any {
			d := good(in)
			d[0].Embedding[2] = 1e39
			return map[string]any{"data": d}
		}, "non-finite"},
		{"fewer embeddings", func(in []string) any { return map[string]any{"data": good(in)[1:]} }, "returned"},
		{"duplicate index", func(in []string) any {
			d := good(in)
			d[1].Index = 0
			return map[string]any{"data": d}
		}, "index"},
		{"empty vector", func(in []string) any {
			d := good(in)
			for i := range d {
				d[i].Embedding = nil
			}
			return map[string]any{"data": d}
		}, "dimension"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newFake(t)
			srv.reply = c.reply
			root := t.TempDir()
			in, out := filepath.Join(root, "in"), filepath.Join(root, "out")
			writeExport(t, in)
			o := baseOptions(srv, in, out)
			o.Which, o.Batch, o.Par = "queries", 4, 1
			_, err := Embed(t.Context(), o, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want mention of %q", err, c.want)
			}
			if _, err := os.Stat(filepath.Join(out, "queries.f32")); err == nil {
				t.Errorf("partial queries.f32 left behind")
			}
		})
	}
}

func TestEmbedInputErrors(t *testing.T) {
	srv := newFake(t)
	root := t.TempDir()
	in := filepath.Join(root, "in")
	writeExport(t, in)
	if err := os.WriteFile(filepath.Join(in, "dup.jsonl"), []byte(`{"key":"a","text":"x"}`+"\n"+`{"key":"a","text":"y"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "empty.jsonl"), []byte(`{"key":"a","text":""}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		mod  func(*Options)
		want string
	}{
		{"which", func(o *Options) { o.Which = "all" }, "--which"},
		{"batch", func(o *Options) { o.Batch = 0 }, "--batch"},
		{"par", func(o *Options) { o.Par = 0 }, "--par"},
		{"duplicate keys", func(o *Options) { o.Which, o.DocsFile = "docs", filepath.Join(in, "dup.jsonl") }, "duplicate"},
		{"empty text", func(o *Options) { o.Which, o.DocsFile = "docs", filepath.Join(in, "empty.jsonl") }, "empty"},
		{"missing input", func(o *Options) { o.In = filepath.Join(root, "nowhere") }, "nowhere"},
		{"url", func(o *Options) { o.URL = "" }, "--url"},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := baseOptions(srv, in, filepath.Join(root, "o-"+strings.ReplaceAll(c.name, " ", "-")))
			c.mod(&o)
			if _, err := Embed(t.Context(), o, nil); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestExpandEscapes(t *testing.T) {
	if got := ExpandEscapes(`Instruct: find\nQuery: `); got != "Instruct: find\nQuery: " {
		t.Errorf("%q", got)
	}
	if got := ExpandEscapes(`a\tb\\n`); got != "a\tb\\n" {
		t.Errorf("%q", got)
	}
}

func TestEmbedErrorBodyNeverEchoesInput(t *testing.T) {
	texts := []string{
		"Заголовок\n\nТело записи о настройке сервера и ZFS, секретные подробности внутри",
		"short title\n\nBody line one has a private hostname grate.internal and a long tail of words",
		strings.Repeat("filler words ", 8) + "needle phrase that only occurs in the middle of this input" + strings.Repeat(" more filler", 8),
	}
	middle := func(s string) string { return s[len(s)/2 : len(s)/2+30] }
	echoes := map[string]func(s string) string{
		"json escaped": func(s string) string {
			b, _ := json.Marshal(map[string]string{"error": "bad input " + s})
			return string(b)
		},
		"ascii escaped": func(s string) string { return `{"error":` + strconvQuoteASCII("bad input "+s) + `}` },
		"middle only":   func(s string) string { return `{"error":"parse error near '` + middle(s) + `'"}` },
		"after prefix":  func(s string) string { return "x" + strings.Repeat(" ", 150) + s },
	}
	for name, echo := range echoes {
		for i, text := range texts {
			t.Run(name+"/"+fmt.Sprint(i), func(t *testing.T) {
				srv := newFake(t)
				srv.fail = func(int32) (int, string) { return 400, echo(text) }
				root := t.TempDir()
				in := filepath.Join(root, "in")
				if err := os.MkdirAll(in, 0o755); err != nil {
					t.Fatal(err)
				}
				var q bytes.Buffer
				for j, s := range texts {
					_ = json.NewEncoder(&q).Encode(map[string]string{"id": fmt.Sprintf("s:%d", j), "text": s})
				}
				if err := os.WriteFile(filepath.Join(in, "queries.jsonl"), q.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
				o := baseOptions(srv, in, filepath.Join(root, "out"))
				o.Which, o.Batch, o.Par = "queries", 8, 1
				_, err := Embed(t.Context(), o, nil)
				if err == nil || !strings.Contains(err.Error(), "400") {
					t.Fatalf("400: %v", err)
				}
				for _, s := range texts {
					for k := 0; k+16 <= len(s); k++ {
						if frag := s[k : k+16]; strings.Contains(err.Error(), frag) {
							t.Fatalf("error repeats input fragment %q: %v", frag, err)
						}
					}
				}
			})
		}
	}
}

func TestEmbedErrorBodyStaysVisibleWhenItDoesNotEcho(t *testing.T) {
	srv := newFake(t)
	srv.fail = func(int32) (int, string) {
		return 413, `{"error":"Input validation error: ` + "`inputs`" + ` must have less than 512 tokens. Given: 3000","error_type":"Validation"}`
	}
	root := t.TempDir()
	in := filepath.Join(root, "in")
	writeExport(t, in)
	o := baseOptions(srv, in, filepath.Join(root, "out"))
	o.Which, o.Batch, o.Par = "queries", 100, 1
	_, err := Embed(t.Context(), o, nil)
	if err == nil || !strings.Contains(err.Error(), "less than 512 tokens") {
		t.Fatalf("diagnostic body lost: %v", err)
	}
}

func strconvQuoteASCII(s string) string { return strconv.QuoteToASCII(s) }

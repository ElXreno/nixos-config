package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

func fakeEmbedServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i, s := range req.Input {
			if seen != nil {
				*seen = append(*seen, s)
			}
			sum := sha256.Sum256([]byte(s))
			v := make([]float64, 6)
			for j := range v {
				v[j] = float64(int(sum[j])-100) / 50
			}
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func exportDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var d, q bytes.Buffer
	for i := range 9 {
		_ = json.NewEncoder(&d).Encode(map[string]string{"key": fmt.Sprintf("k%d", i), "text": fmt.Sprintf("secret document body %d", i)})
	}
	for i := range 5 {
		_ = json.NewEncoder(&q).Encode(map[string]string{"id": fmt.Sprintf("s:%d", i), "session": "s", "set": "gold", "text": fmt.Sprintf("secret prompt %d", i)})
	}
	for name, b := range map[string]*bytes.Buffer{"docs.jsonl": &d, "queries.jsonl": &q} {
		if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmbedCommand(t *testing.T) {
	sb := newSandbox(t)
	var seen []string
	srv := fakeEmbedServer(t, &seen)
	in, out := filepath.Join(sb.dir, "in"), filepath.Join(sb.dir, "out")
	exportDir(t, in)
	code, stdout, stderr := sb.run("", "embed", "--url", srv.URL, "--model", "fake", "--query-prefix", `Instruct: find\nQuery: `, "--doc-prefix", "passage: ",
		"--in", in, "--out", out, "--batch", "4", "--par", "2")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, w := range []string{"docs 9", "queries 5", "dim 6", "texts/s", out} {
		if !strings.Contains(stdout, w) {
			t.Errorf("stdout lacks %q:\n%s", w, stdout)
		}
	}
	if strings.Contains(stdout+stderr, "secret") {
		t.Errorf("output leaks text:\n%s%s", stdout, stderr)
	}
	foundQ := false
	for _, s := range seen {
		if strings.HasPrefix(s, "Instruct: find\nQuery: secret prompt") {
			foundQ = true
		}
	}
	if !foundQ {
		t.Errorf("query prefix escape not expanded: %q", seen)
	}
	for _, name := range []string{"docs.f32", "docs.keys", "queries.f32", "queries.ids", "meta.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Error(err)
		}
	}
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"embed", "--model", "m", "--in", in, "--out", out}, 2, "--url"},
		{[]string{"embed", "--url", srv.URL, "--in", in, "--out", out}, 2, "--model"},
		{[]string{"embed", "--url", srv.URL, "--model", "m", "--in", in, "--out", out, "--which", "x"}, 2, "--which"},
		{[]string{"embed", "--url", srv.URL, "--model", "m", "--in", in, "--out", out, "extra"}, 2, "unexpected"},
		{[]string{"embed", "--url", "http://127.0.0.1:1", "--model", "m", "--in", in, "--out", filepath.Join(sb.dir, "o2"), "--retries", "0"}, 1, "127.0.0.1"},
	} {
		code, _, stderr := sb.run("", c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Errorf("%v = %d %q, want %d %q", c.args, code, stderr, c.code, c.want)
		}
	}
}

func TestVecdiffCommand(t *testing.T) {
	sb := newSandbox(t)
	srv := fakeEmbedServer(t, nil)
	in := filepath.Join(sb.dir, "in")
	exportDir(t, in)
	a, b := filepath.Join(sb.dir, "a"), filepath.Join(sb.dir, "b")
	for _, out := range []string{a, b} {
		if code, _, stderr := sb.run("", "embed", "--url", srv.URL, "--model", "fake", "--in", in, "--out", out); code != 0 {
			t.Fatalf("embed: %s", stderr)
		}
	}
	code, stdout, stderr := sb.run("", "vecdiff", a, b)
	if code != 0 || !strings.Contains(stdout, "docs") || !strings.Contains(stdout, "count 9") || !strings.Contains(stdout, "queries") || !strings.Contains(stdout, "min 1.0000") {
		t.Fatalf("exit %d\n%s%s", code, stdout, stderr)
	}
	code, stdout, _ = sb.run("", "vecdiff", a, b, "--json", "--which", "docs")
	var res []struct {
		Part  string  `json:"part"`
		Count int     `json:"count"`
		Mean  float64 `json:"mean"`
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &res) != nil || len(res) != 1 || res[0].Part != "docs" || res[0].Count != 9 || res[0].Mean < 0.9999 {
		t.Errorf("json: %d %s", code, stdout)
	}
	if code, _, stderr := sb.run("", "vecdiff", a); code != 2 || !strings.Contains(stderr, "two") {
		t.Errorf("one dir: %d %q", code, stderr)
	}
	if code, _, _ := sb.run("", "vecdiff", a, filepath.Join(sb.dir, "none")); code != 1 {
		t.Errorf("missing dir: %d", code)
	}
}

func TestBuildENCommand(t *testing.T) {
	sb := newSandbox(t)
	db := filepath.Join(sb.dir, "ru.db")
	s, err := store.Open(t.Context(), db, store.Options{Now: func() time.Time { return sb.now }})
	if err != nil {
		t.Fatal(err)
	}
	title, body := "Заголовок", "Тело записи про кулер"
	if _, err := s.Put(t.Context(), store.PutRequest{Key: "k1", Kind: store.Rule, Scopes: []string{store.GlobalScope}, Title: &title, Body: &body, Gate: store.DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	en := filepath.Join(sb.dir, "en.jsonl")
	if err := os.WriteFile(en, []byte(`{"id":1,"key":"k1","title_en":"Title","body_en":"Body about the cooler","issues":null}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(sb.dir, "en.db")
	code, stdout, stderr := sb.run("", "build-en", "--db", db, "--en", en, "--out", out)
	if code != 0 || !strings.Contains(stdout, "records 1, replaced 1, missing 0") {
		t.Fatalf("exit %d %q %q", code, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "cooler") {
		t.Error("output leaks record text")
	}
	if code, _, stderr := sb.run("", "build-en", "--db", db, "--en", en, "--out", out); code != 1 || !strings.Contains(stderr, "exists") {
		t.Errorf("rerun: %d %q", code, stderr)
	}
	if code, _, stderr := sb.run("", "build-en", "--db", db, "--en", en); code != 2 || !strings.Contains(stderr, "--out") {
		t.Errorf("no out: %d %q", code, stderr)
	}
}

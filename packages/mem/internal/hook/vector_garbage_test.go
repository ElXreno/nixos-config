package hook

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

type garbageServer struct {
	name    string
	counter string
	handler http.HandlerFunc
}

func rawReply(status int, ctype, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func hijackThen(fn func(c net.Conn, rw *bufio.ReadWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		fn(c, rw)
	}
}

func garbageServers() []garbageServer {
	json200 := func(body string) http.HandlerFunc { return rawReply(200, "application/json", body) }
	return []garbageServer{
		{"html page", "response", rawReply(200, "text/html", "<html><body>502 from a captive portal</body></html>")},
		{"empty body", "response", json200("")},
		{"json null", "response", json200("null")},
		{"json array", "response", json200("[1,2,3]")},
		{"no data", "response", json200(`{"data":[]}`)},
		{"data is not a list", "response", json200(`{"data":"nope"}`)},
		{"two vectors for one text", "response", json200(`{"data":[{"index":0,"embedding":[1,0,0,0,0]},{"index":1,"embedding":[1,0,0,0,0]}]}`)},
		{"index out of range", "response", json200(`{"data":[{"index":7,"embedding":[1,0,0,0,0]}]}`)},
		{"negative index", "response", json200(`{"data":[{"index":-1,"embedding":[1,0,0,0,0]}]}`)},
		{"embedding is a string", "response", json200(`{"data":[{"index":0,"embedding":"abc"}]}`)},
		{"embedding holds nulls", "response", json200(`{"data":[{"index":0,"embedding":[null,null,null,null,null]}]}`)},
		{"embedding missing", "response", json200(`{"data":[{"index":0}]}`)},
		{"embedding empty", "response", json200(`{"data":[{"index":0,"embedding":[]}]}`)},
		{"number out of float64 range", "response", json200(`{"data":[{"index":0,"embedding":[1e999,0,0,0,0]}]}`)},
		{"number out of float32 range", "response", json200(`{"data":[{"index":0,"embedding":[1e300,0,0,0,0]}]}`)},
		{"all zero", "response", json200(`{"data":[{"index":0,"embedding":[0,0,0,0,0]}]}`)},
		{"denormals only", "response", json200(`{"data":[{"index":0,"embedding":[1e-300,1e-300,1e-300,1e-300,1e-300]}]}`)},
		{"wrong dimension", "dim", json200(`{"data":[{"index":0,"embedding":[1,0,0]}]}`)},
		{"truncated json", "response", json200(`{"data":[{"index":0,"embedding":[1,0,0,`)},
		{"http 500", "http", rawReply(500, "text/plain", "boom")},
		{"http 503", "http", rawReply(503, "text/plain", "loading model")},
		{"http 429", "http", rawReply(429, "text/plain", "slow down")},
		{"http 400", "http", rawReply(400, "text/plain", "input is too large")},
		{"http 404", "http", rawReply(404, "text/plain", "no such route")},
		{"redirect loop", "response", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, r.URL.Path, http.StatusTemporaryRedirect)
		}},
		{"gzip header over plain body", "response", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0,0,0,0]}]}`))
		}},
		{"connection closed at once", "response", hijackThen(func(c net.Conn, rw *bufio.ReadWriter) {})},
		{"half a status line", "response", hijackThen(func(c net.Conn, rw *bufio.ReadWriter) { _, _ = rw.WriteString("HTTP/1.1 200 O"); _ = rw.Flush() })},
		{"body shorter than content-length", "response", hijackThen(func(c net.Conn, rw *bufio.ReadWriter) {
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 500\r\n\r\n{\"data\":[")
			_ = rw.Flush()
		})},
		{"never answers", "timeout", hijackThen(func(c net.Conn, rw *bufio.ReadWriter) { time.Sleep(3 * time.Second) })},
		{"headers then a stalled body", "timeout", hijackThen(func(c net.Conn, rw *bufio.ReadWriter) {
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 400\r\n\r\n{\"data\":[")
			_ = rw.Flush()
			time.Sleep(3 * time.Second)
		})},
		{"body dripped a byte at a time", "timeout", hijackThen(func(c net.Conn, rw *bufio.ReadWriter) {
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n")
			_ = rw.Flush()
			for i := 0; i < 100; i++ {
				if _, err := rw.WriteString("1\r\n \r\n"); err != nil {
					return
				}
				if rw.Flush() != nil {
					return
				}
				time.Sleep(40 * time.Millisecond)
			}
		})},
	}
}

func TestVectorGarbageRepliesFallBackToLexical(t *testing.T) {
	const prompt = "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	ref := newSandbox(t)
	ref.seed(t)
	ref.indexVectors(t)
	_, want, _ := ref.run(promptInput(t, ref, "lex", prompt), UserPromptSubmit)
	if want == "" {
		t.Fatal("the lexical reference run injected nothing")
	}
	for _, g := range garbageServers() {
		t.Run(g.name, func(t *testing.T) {
			srv := httptest.NewServer(g.handler)
			defer srv.Close()
			sb := newSandbox(t)
			sb.seed(t)
			sb.indexVectors(t)
			args := append([]string{UserPromptSubmit}, vecFlags...)
			args[2] = srv.URL
			start := time.Now()
			code, got, errs := sb.run(promptInput(t, sb, "lex", prompt), args...)
			if code != 0 || errs != "" {
				t.Errorf("exit %d, stderr %q", code, errs)
			}
			if withoutNotice(t, got) != want || !strings.Contains(got, noticeLead) {
				t.Errorf("stdout differs from the lexical path plus the notice:\n%s\nvs\n%s", got, want)
			}
			if d := time.Since(start); d > 1200*time.Millisecond {
				t.Errorf("took %v", d)
			}
			counts := sb.counters(t)
			if counts[g.counter] != 1 || len(counts) != 1 {
				t.Errorf("counters %v, want %s=1 (log %q)", counts, g.counter, sb.lastLog(t))
			}
			if line := sb.lastLog(t); !strings.Contains(line, "embed="+g.counter) || !strings.Contains(line, "inject") {
				t.Errorf("log line %q", line)
			}
		})
	}
}

func TestVectorHugeReplyIsNotBuffered(t *testing.T) {
	const prompt = "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		chunk := []byte(strings.Repeat("a", 1<<16))
		for {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	args := append([]string{UserPromptSubmit}, vecFlags...)
	args[2] = srv.URL
	in := promptInput(t, sb, "huge", prompt)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	code, got, _ := sb.run(in, args...)
	runtime.ReadMemStats(&after)
	if code != 0 || got == "" {
		t.Errorf("exit %d, stdout %q", code, got)
	}
	if grown := (after.TotalAlloc - before.TotalAlloc) >> 20; grown > 64 {
		t.Errorf("the hook allocated %d MiB reading a reply that never ends", grown)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "embed=") || strings.Contains(line, "embed=ok") {
		t.Errorf("log line %q", line)
	}
}

type panicEmbedder struct{}

func (panicEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	var empty [][]float32
	return [][]float32{empty[3]}, nil
}

type stubbornEmbedder struct{ hold time.Duration }

func (s stubbornEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	time.Sleep(s.hold)
	return nil, errors.New("late")
}

func TestVectorEmbedderPanicFallsBack(t *testing.T) {
	const prompt = "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	ref := newSandbox(t)
	ref.seed(t)
	_, want, _ := ref.run(promptInput(t, ref, "lex", prompt), UserPromptSubmit)
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, panicEmbedder{})
	code, got, _ := sb.run(promptInput(t, sb, "lex", prompt), append([]string{UserPromptSubmit}, vecFlags...)...)
	if code != 0 || withoutNotice(t, got) != want || !strings.Contains(got, noticeLead) {
		t.Errorf("exit %d, stdout %q, want %q and the notice", code, got, want)
	}
	if c := sb.counters(t); c["error"] != 1 || len(c) != 1 {
		t.Errorf("counters %v", c)
	}
}

func TestVectorEmbedderIgnoringItsContextIsStillBounded(t *testing.T) {
	const prompt = "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	ref := newSandbox(t)
	ref.seed(t)
	_, want, _ := ref.run(promptInput(t, ref, "lex", prompt), UserPromptSubmit)
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, stubbornEmbedder{hold: 4 * time.Second})
	start := time.Now()
	code, got, _ := sb.run(promptInput(t, sb, "lex", prompt), append(append([]string{UserPromptSubmit}, vecFlags...), "--embed-timeout", "100ms")...)
	if d := time.Since(start); d > 800*time.Millisecond {
		t.Errorf("took %v with an embed timeout of 100ms", d)
	}
	if code != 0 || withoutNotice(t, got) != want || !strings.Contains(got, noticeLead) {
		t.Errorf("exit %d, stdout %q, want %q and the notice", code, got, want)
	}
	if c := sb.counters(t); c["timeout"] != 1 || len(c) != 1 {
		t.Errorf("counters %v", c)
	}
}

func TestVectorEmbedTimeoutLongerThanTheBudgetKeepsTheLexicalAnswer(t *testing.T) {
	const prompt = "Can I git push the commit to the remote now, or do I wait for your authorization first?"
	ref := newSandbox(t)
	ref.seed(t)
	_, want, _ := ref.run(promptInput(t, ref, "lex", prompt), UserPromptSubmit)
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{delay: 5 * time.Second})
	start := time.Now()
	code, got, _ := sb.run(promptInput(t, sb, "lex", prompt), append(append([]string{UserPromptSubmit}, vecFlags...), "--embed-timeout", "10s", "--budget", "600ms")...)
	if d := time.Since(start); d > 900*time.Millisecond {
		t.Errorf("took %v with a budget of 600ms", d)
	}
	if code != 0 || withoutNotice(t, got) != want || !strings.Contains(got, noticeLead) {
		t.Errorf("exit %d, stdout %q, want the lexical %q and the notice", code, got, want)
	}
}

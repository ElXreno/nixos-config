package vec

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newClient(url string) *Client {
	return NewClient(Options{URL: url, Model: "fake", Timeout: 2 * time.Second})
}

func TestClientEmbedNormalizes(t *testing.T) {
	f := newFake(t)
	out, err := newClient(f.URL).Embed(context.Background(), []string{"alpha", "beta", "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("%d vectors for 3 texts", len(out))
	}
	for i, v := range out {
		if len(v) != testDim {
			t.Errorf("vector %d has dimension %d, want %d", i, len(v), testDim)
		}
		if n := Dot(v, v); math.Abs(n-1) > 1e-5 {
			t.Errorf("vector %d has squared norm %v, want 1", i, n)
		}
	}
	if len(f.inputs) != 3 {
		t.Errorf("server saw %d inputs", len(f.inputs))
	}
}

func TestClientRejectsBadVectors(t *testing.T) {
	zero := make([]float64, testDim)
	huge := make([]float64, testDim)
	huge[0] = 1e39
	cases := []struct {
		name  string
		reply func(inputs []string) any
	}{
		{"zero vector", func(in []string) any { return embedBody(in, func(int) []float64 { return zero }) }},
		{"overflow to infinity", func(in []string) any { return embedBody(in, func(int) []float64 { return huge }) }},
		{"empty vector", func(in []string) any { return embedBody(in, func(int) []float64 { return nil }) }},
		{"mixed dimensions", func(in []string) any {
			return embedBody(in, func(i int) []float64 { return fakeVec("x", testDim+i) })
		}},
		{"too few vectors", func(in []string) any { return embedBody(in[:1], func(int) []float64 { return fakeVec("x", testDim) }) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			f.reply = c.reply
			out, err := newClient(f.URL).Embed(context.Background(), []string{"a", "b"})
			if err == nil {
				t.Fatalf("accepted %v", out)
			}
			if got := Classify(err); got != FailResponse {
				t.Errorf("classified as %q (%v), want %q", got, err, FailResponse)
			}
		})
	}
}

func embedBody(inputs []string, vec func(i int) []float64) any {
	type item struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	}
	var data []item
	for i := range inputs {
		data = append(data, item{i, vec(i)})
	}
	return map[string]any{"data": data}
}

func TestClassify(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(hang))
	defer slow.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, strings.Repeat("x", 10), http.StatusInternalServerError)
	}))
	defer broken.Close()
	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("NaN")) }))
	defer garbage.Close()
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"context deadline", slow.URL, FailTimeout},
		{"connection refused", downURL, FailDown},
		{"server error", broken.URL, FailHTTP},
		{"not json", garbage.URL, FailResponse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := newClient(c.url).Embed(ctx, []string{"a"})
			if err == nil {
				t.Fatal("no error")
			}
			if got := Classify(err); got != c.want {
				t.Errorf("classified as %q (%v), want %q", got, err, c.want)
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v", d)
			}
		})
	}
	if got := Classify(errors.New("other")); got != FailResponse {
		t.Errorf("unknown error classified as %q", got)
	}
}

func TestClientHTTPClientTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(hang))
	defer slow.Close()
	c := NewClient(Options{URL: slow.URL, Model: "fake", Timeout: 80 * time.Millisecond})
	_, err := c.Embed(context.Background(), []string{"a"})
	if got := Classify(err); got != FailTimeout {
		t.Errorf("classified as %q (%v), want %q", got, err, FailTimeout)
	}
}

func hang(_ http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(10 * time.Second):
	}
}

package vec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientCapsTheReply(t *testing.T) {
	endless := func(status int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			chunk := []byte(strings.Repeat("a", 1<<14))
			for {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		}))
	}
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"ok status", 200, FailResponse},
		{"error status", 500, FailHTTP},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := endless(c.status)
			defer srv.Close()
			cl := NewClient(Options{URL: srv.URL, Model: "m", MaxBody: 1 << 16, Timeout: 5 * time.Second})
			start := time.Now()
			_, err := cl.Embed(context.Background(), []string{"x"})
			if err == nil {
				t.Fatal("no error")
			}
			if got := Classify(err); got != c.want {
				t.Errorf("classified %q, want %q (%v)", got, c.want, err)
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v", d)
			}
		})
	}
}

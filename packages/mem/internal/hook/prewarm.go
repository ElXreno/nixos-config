package hook

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"syscall"
	"time"

	"mem/internal/vec"
)

const (
	prewarmDial    = 100 * time.Millisecond
	prewarmOK      = "ok"
	prewarmSkipped = "skip"
	prewarmError   = "error"
)

var prewarmControl func(ctx context.Context, network, address string, c syscall.RawConn) error

func prewarmAddr(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", false
		}
	}
	return net.JoinHostPort(u.Hostname(), port), true
}

func prewarmAll(ctx context.Context, embedURL, rerankURL string) []string {
	targets := []struct{ name, url string }{{"prewarm", embedURL}, {"prewarm-rerank", rerankURL}}
	notes := make([]string, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() {
			start := time.Now()
			if outcome := prewarm(ctx, t.url); outcome != "" {
				notes[i] = fmt.Sprintf("%s=%s %dms", t.name, outcome, time.Since(start).Milliseconds())
			}
		})
	}
	wg.Wait()
	return nonEmpty(notes...)
}

func prewarm(ctx context.Context, rawURL string) string {
	if rawURL == "" {
		return ""
	}
	addr, ok := prewarmAddr(rawURL)
	if !ok {
		return prewarmSkipped
	}
	d := net.Dialer{Timeout: prewarmDial, ControlContext: prewarmControl}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		switch o := vec.Classify(err); o {
		case vec.FailTimeout, vec.FailDown:
			return o
		}
		return prewarmError
	}
	_ = c.Close()
	return prewarmOK
}

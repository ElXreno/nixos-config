package vec

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

const pickDial = 150 * time.Millisecond

func Split(raw string) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func hostPort(raw string) (string, bool) {
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

func Pick(ctx context.Context, raw string) string {
	urls := Split(raw)
	switch len(urls) {
	case 0:
		return ""
	case 1:
		return urls[0]
	}
	d := net.Dialer{Timeout: pickDial}
	for _, u := range urls[:len(urls)-1] {
		addr, ok := hostPort(u)
		if !ok {
			continue
		}
		c, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			continue
		}
		_ = c.Close()
		return u
	}
	return urls[len(urls)-1]
}

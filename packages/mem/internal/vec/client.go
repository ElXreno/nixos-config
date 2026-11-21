package vec

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

const (
	FailTimeout  = "timeout"
	FailDown     = "down"
	FailHTTP     = "http"
	FailResponse = "response"
)

var ErrBadVector = errors.New("unusable embedding")

type Client struct{ c *client }

func NewClient(o Options) *Client {
	if o.Backoff == 0 {
		o.Backoff = time.Second
	}
	if o.Timeout == 0 {
		o.Timeout = 10 * time.Minute
	}
	urls := order(context.Background(), o.URL)
	c := &client{o: o, http: o.Client, url: urls[0], urls: urls}
	if c.http == nil {
		c.http = &http.Client{Timeout: o.Timeout}
	}
	return &Client{c: c}
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out, err := c.c.embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	dim := 0
	for i, v := range out {
		if len(v) == 0 {
			return nil, fmt.Errorf("%w: text %d has an empty vector", ErrBadVector, i)
		}
		if dim == 0 {
			dim = len(v)
		}
		if len(v) != dim {
			return nil, fmt.Errorf("%w: dimension %d against %d in one response", ErrBadVector, len(v), dim)
		}
		if err := Normalize(v); err != nil {
			return nil, fmt.Errorf("%w: text %d: %v", ErrBadVector, i, err)
		}
	}
	return out, nil
}

func Classify(err error) string {
	var he *httpError
	var ne net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return FailTimeout
	case errors.As(err, &he):
		return FailHTTP
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return FailDown
	}
	var oe *net.OpError
	if errors.As(err, &oe) {
		return FailDown
	}
	return FailResponse
}

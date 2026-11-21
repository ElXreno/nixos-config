package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newStore(t *testing.T) (*Store, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "mem.db"), Options{Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, c
}

func ptr[T any](v T) *T { return &v }

type spec struct {
	key, title, body string
	kind             Kind
	scopes           []string
	core             bool
}

func (sp spec) req() PutRequest {
	kind := sp.kind
	if kind == "" {
		kind = Rule
	}
	scopes := sp.scopes
	if scopes == nil {
		scopes = []string{GlobalScope}
	}
	return PutRequest{
		Key:    sp.key,
		Kind:   kind,
		Scopes: scopes,
		Title:  ptr(sp.title),
		Body:   ptr(sp.body),
		Core:   ptr(sp.core),
		Gate:   DefaultGate(),
	}
}

func mustPut(t *testing.T, s *Store, req PutRequest) PutResult {
	t.Helper()
	res, err := s.Put(context.Background(), req)
	if err != nil {
		t.Fatalf("put %s: %v", req.Key, err)
	}
	return res
}

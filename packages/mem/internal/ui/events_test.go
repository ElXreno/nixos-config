package ui

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mem/internal/statefile"
	"mem/internal/store"
)

type sseEvent struct {
	name, id, data string
}

type sseClient struct {
	resp   *http.Response
	events chan sseEvent
	pings  atomic.Int64
	done   chan struct{}
}

func openSSE(t *testing.T, base string) *sseClient {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("events = %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	c := &sseClient{resp: resp, events: make(chan sseEvent, 256), done: make(chan struct{})}
	go func() {
		defer close(c.done)
		defer close(c.events)
		sc := bufio.NewScanner(resp.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if ev.name != "" {
					c.events <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, ":"):
				c.pings.Add(1)
			case strings.HasPrefix(line, "event: "):
				ev.name = line[7:]
			case strings.HasPrefix(line, "id: "):
				ev.id = line[4:]
			case strings.HasPrefix(line, "data: "):
				ev.data = line[6:]
			}
		}
	}()
	t.Cleanup(func() { _ = resp.Body.Close() })
	return c
}

func (c *sseClient) next(t *testing.T, name string, within time.Duration) sseEvent {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case ev, ok := <-c.events:
			if !ok {
				t.Fatalf("stream closed while waiting for %q", name)
			}
			if ev.name == name {
				return ev
			}
		case <-deadline:
			t.Fatalf("no %q event within %s", name, within)
		}
	}
}

func eventFixture(t *testing.T, state string) (*fixture, *httptest.Server) {
	t.Helper()
	f := newFixture(t)
	srv := New(f.st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }, StateDir: state}, false)
	srv.ev.every = 40 * time.Millisecond
	srv.ev.beat = 120 * time.Millisecond
	f = &fixture{st: f.st, srv: srv, h: srv.Handler()}
	ts := httptest.NewUnstartedServer(f.h)
	ts.Start()
	t.Cleanup(ts.Close)
	return f, ts
}

func TestEventsEmitAfterPut(t *testing.T) {
	f, ts := eventFixture(t, "")
	c := openSSE(t, ts.URL)
	first := c.next(t, "graph", 5*time.Second)
	if !strings.Contains(first.data, `"rev":"`) {
		t.Fatalf("first graph event = %q", first.data)
	}
	var cur struct {
		Rev string `json:"rev"`
	}
	f.json(t, "/api/graph", &cur)
	if !strings.Contains(first.data, cur.Rev) {
		t.Fatalf("hello rev %s is not the served rev %s", first.data, cur.Rev)
	}
	put(t, f.st, store.PutRequest{Key: "reference_sse", Kind: store.Fact, Title: sp("Sse"), Body: sp("Sse body.")})
	ev := c.next(t, "graph", 5*time.Second)
	if ev.data == first.data {
		t.Fatalf("a put re-announced revision %s", ev.data)
	}
	var diff struct {
		Add []struct {
			K string `json:"k"`
		} `json:"add"`
	}
	f.json(t, "/api/graph/diff?from="+cur.Rev, &diff)
	if len(diff.Add) != 1 || diff.Add[0].K != "reference_sse" {
		t.Fatalf("diff after the event = %+v", diff.Add)
	}
}

func TestEventsDeliverTurns(t *testing.T) {
	dir := t.TempDir()
	_, ts := eventFixture(t, dir)
	c := openSSE(t, ts.URL)
	c.next(t, "graph", 5*time.Second)
	time.Sleep(150 * time.Millisecond)
	line := statefile.Turn{Time: "2026-11-07T11:00:00Z", Event: statefile.EventPrompt, Session: "sess-1", PromptSHA: "0123456789abcdef", Prompt: "hello", Status: "inject", Strong: []statefile.Key{{Key: "feedback_alpha", Cos: .8}}}
	if err := statefile.Append(dir, statefile.TurnsName, 1<<20, line); err != nil {
		t.Fatal(err)
	}
	ev := c.next(t, "turn", 5*time.Second)
	for _, want := range []string{`"id":"`, `"session":"sess-1"`, `"feedback_alpha"`, `"origin":"turns"`} {
		if !strings.Contains(ev.data, want) {
			t.Fatalf("turn event lacks %s: %s", want, ev.data)
		}
	}
	if _, err := statefile.ParseCursor(ev.id); err != nil {
		t.Fatalf("turn event id %q is not a cursor: %v", ev.id, err)
	}
}

func TestEventsCheckOnlyWhileSubscribed(t *testing.T) {
	f, ts := eventFixture(t, "")
	hub := f.srv.ev
	time.Sleep(200 * time.Millisecond)
	if n := hub.checks.Load(); n != 0 {
		t.Fatalf("the fingerprint was checked %d times with nobody subscribed", n)
	}
	c := openSSE(t, ts.URL)
	c.next(t, "graph", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if n := hub.checks.Load(); n < 3 {
		t.Fatalf("only %d checks in 300 ms with a subscriber", n)
	}
	if hub.count() != 1 {
		t.Fatalf("subscribers = %d", hub.count())
	}
	_ = c.resp.Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for hub.count() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.count() != 0 {
		t.Fatal("the subscriber was not dropped after the client left")
	}
	time.Sleep(100 * time.Millisecond)
	after := hub.checks.Load()
	time.Sleep(300 * time.Millisecond)
	if hub.checks.Load() != after {
		t.Fatalf("checks went on after the last subscriber left: %d -> %d", after, hub.checks.Load())
	}
}

func TestEventsHeartbeatBeatsServerTimeouts(t *testing.T) {
	f := newFixture(t)
	srv := New(f.st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }}, false)
	srv.ev.beat = 100 * time.Millisecond
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Config.ReadTimeout = 400 * time.Millisecond
	ts.Config.WriteTimeout = 500 * time.Millisecond
	ts.Start()
	t.Cleanup(ts.Close)
	c := openSSE(t, ts.URL)
	c.next(t, "graph", 5*time.Second)
	time.Sleep(2500 * time.Millisecond)
	select {
	case <-c.done:
		t.Fatal("the stream was cut by the server timeouts")
	default:
	}
	if n := c.pings.Load(); n < 15 {
		t.Fatalf("%d heartbeats in 2.5 s at 100 ms", n)
	}
	if rec, err := http.Get(ts.URL + "/api/stats"); err != nil || rec.StatusCode != http.StatusOK {
		t.Fatalf("other requests broke: %v", err)
	}
}

func TestEventsDropSlowSubscriber(t *testing.T) {
	f := newFixture(t)
	hub := f.srv.ev
	b := hub.subscribe()
	if b == nil {
		t.Fatal("subscribe failed")
	}
	defer hub.unsubscribe(b)
	for i := 0; i < eventBuffer+2; i++ {
		hub.broadcast(evMsg{name: "graph", data: []byte(strconv.Itoa(i))})
	}
	select {
	case <-b.dead:
	default:
		t.Fatal("a subscriber with a full buffer was not dropped")
	}
}

func TestEventsRefuseTooManyStreams(t *testing.T) {
	f := newFixture(t)
	hub := f.srv.ev
	var subs []*evSub
	for i := 0; i < maxEventStream; i++ {
		b := hub.subscribe()
		if b == nil {
			t.Fatalf("subscriber %d refused", i)
		}
		subs = append(subs, b)
	}
	if hub.subscribe() != nil {
		t.Fatal("subscriber over the limit accepted")
	}
	rec := f.do(t, http.MethodGet, "/api/events")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("over the limit = %d", rec.Code)
	}
	for _, b := range subs {
		hub.unsubscribe(b)
	}
	if hub.count() != 0 {
		t.Fatalf("subscribers left: %d", hub.count())
	}
}

func TestEventsStreamEndsOnShutdown(t *testing.T) {
	f := newFixture(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, f.srv.Handler()) }()
	c := openSSE(t, "http://"+ln.Addr().String())
	c.next(t, "graph", 5*time.Second)
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve with an open stream returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return with an open stream")
	}
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Fatalf("shutdown took %s with a stream open", d)
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("the client stream stayed open after shutdown")
	}
}

func TestEventsAreGetOnly(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/api/events", nil)
	req.Host = "127.0.0.1:7781"
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/events = %d", rec.Code)
	}
}

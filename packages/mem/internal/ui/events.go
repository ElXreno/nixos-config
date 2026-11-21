package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mem/internal/statefile"
)

const (
	eventTick      = time.Second
	eventBeat      = 15 * time.Second
	eventWrite     = 10 * time.Second
	eventRetryMs   = 3000
	eventBuffer    = 64
	maxEventStream = 32
)

type evMsg struct {
	name string
	id   string
	data []byte
}

type evSub struct {
	ch   chan evMsg
	dead chan struct{}
	once sync.Once
}

func (b *evSub) kill() { b.once.Do(func() { close(b.dead) }) }

type eventHub struct {
	s      *Server
	every  time.Duration
	beat   time.Duration
	checks atomic.Int64

	mu     sync.Mutex
	subs   map[*evSub]struct{}
	cancel context.CancelFunc
	closed bool
}

func newEventHub(s *Server) *eventHub {
	return &eventHub{s: s, every: eventTick, beat: eventBeat, subs: map[*evSub]struct{}{}}
}

func (h *eventHub) subscribe() *evSub {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.subs) >= maxEventStream {
		return nil
	}
	b := &evSub{ch: make(chan evMsg, eventBuffer), dead: make(chan struct{})}
	h.subs[b] = struct{}{}
	if len(h.subs) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		h.cancel = cancel
		go h.run(ctx)
	}
	return b
}

func (h *eventHub) unsubscribe(b *evSub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[b]; !ok {
		return
	}
	delete(h.subs, b)
	b.kill()
	if len(h.subs) == 0 && h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *eventHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for b := range h.subs {
		b.kill()
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *eventHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func (h *eventHub) broadcast(m evMsg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for b := range h.subs {
		select {
		case b.ch <- m:
		default:
			b.kill()
		}
	}
}

type evState struct {
	fp  string
	rev string
	cur statefile.Cursor
}

func (h *eventHub) fpKey(ctx context.Context) (string, error) {
	fp, err := h.s.fingerprint(ctx)
	if err != nil {
		return "", err
	}
	return fp + "|" + h.s.opt.now().Format(time.DateOnly), nil
}

func (h *eventHub) run(ctx context.Context) {
	var st evState
	if fp, err := h.fpKey(ctx); err == nil {
		if snap, err := h.s.revision(ctx); err == nil && snap.rev != "" {
			st.fp, st.rev = fp, snap.rev
		}
	}
	if _, next, err := h.s.readTurns(nil); err == nil {
		st.cur = next
	}
	t := time.NewTicker(h.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.tick(ctx, &st)
		}
	}
}

func (h *eventHub) tick(ctx context.Context, st *evState) {
	h.checks.Add(1)
	if fp, err := h.fpKey(ctx); err == nil && fp != st.fp {
		if snap, err := h.s.revision(ctx); err == nil && snap.rev != "" {
			st.fp = fp
			if snap.rev != st.rev {
				st.rev = snap.rev
				data, _ := json.Marshal(map[string]string{"rev": snap.rev})
				h.broadcast(evMsg{name: "graph", data: data})
			}
		}
	}
	since := st.cur
	turns, next, err := h.s.readTurns(&since)
	if err != nil {
		return
	}
	st.cur = next
	if len(turns) == 0 {
		return
	}
	h.s.joinShadow(turns)
	for i, t := range turns {
		data, err := json.Marshal(t)
		if err != nil {
			continue
		}
		m := evMsg{name: "turn", data: data}
		if i == len(turns)-1 {
			m.id = next.String()
		}
		h.broadcast(m)
	}
}

func sseFrame(m evMsg) []byte {
	var b strings.Builder
	b.WriteString("event: " + m.name + "\n")
	if m.id != "" {
		b.WriteString("id: " + m.id + "\n")
	}
	b.WriteString("data: ")
	b.Write(m.data)
	b.WriteString("\n\n")
	return []byte(b.String())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sub := s.ev.subscribe()
	if sub == nil {
		writeError(w, http.StatusServiceUnavailable, "too many event streams")
		return
	}
	defer s.ev.unsubscribe(sub)
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-store")
	h.Set("X-Accel-Buffering", "no")
	write := func(p []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(eventWrite))
		if _, err := w.Write(p); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	w.WriteHeader(http.StatusOK)
	if !write([]byte("retry: " + strconv.Itoa(eventRetryMs) + "\n\n")) {
		return
	}
	if snap, err := s.revision(r.Context()); err == nil && snap.rev != "" {
		data, _ := json.Marshal(map[string]string{"rev": snap.rev})
		if !write(sseFrame(evMsg{name: "graph", data: data})) {
			return
		}
	}
	beat := time.NewTicker(s.ev.beat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.dead:
			return
		case m := <-sub.ch:
			if !write(sseFrame(m)) {
				return
			}
		case <-beat.C:
			if !write([]byte(": ping\n\n")) {
				return
			}
		}
	}
}

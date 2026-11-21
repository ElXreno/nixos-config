package ui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mem/internal/store"
)

//go:embed assets
var assetFS embed.FS

const (
	DefaultListen = "127.0.0.1:7781"
	CSP           = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'none'; connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'"
)

var ErrNotLoopback = errors.New("refusing a non-loopback address without --allow-remote")

func CheckListen(addr string, allowRemote bool) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address %q: %w", addr, err)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 0 || p > 65535 {
		return fmt.Errorf("listen address %q: port %q is not 0-65535", addr, port)
	}
	if allowRemote {
		return nil
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("%w: %q", ErrNotLoopback, addr)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func CheckBound(ln net.Listener, allowRemote bool) error {
	if allowRemote {
		return nil
	}
	ta, ok := ln.Addr().(*net.TCPAddr)
	if !ok || !ta.IP.IsLoopback() {
		return fmt.Errorf("%w: bound %s", ErrNotLoopback, ln.Addr())
	}
	return nil
}

type Server struct {
	st          *store.Store
	opt         Options
	allowRemote bool

	mu    sync.Mutex
	stamp string
	graph *Graph

	turnMu sync.Mutex
	snap   *turnSnap

	vecMu  sync.Mutex
	vecIdx *vecCache

	probeMu sync.Mutex
	probes  map[string]probeResult

	gate chan struct{}

	lay *layoutMgr
	rv  revStore
	ev  *eventHub
}

func New(st *store.Store, opt Options, allowRemote bool) *Server {
	s := &Server{st: st, opt: opt, allowRemote: allowRemote, gate: make(chan struct{}, maxRemoteCalls), probes: map[string]probeResult{}, lay: newLayoutMgr(opt.StateDir, st.Path(), opt.Relayout)}
	s.ev = newEventHub(s)
	return s
}

type route struct {
	pattern string
	handler func(http.ResponseWriter, *http.Request)
}

func (s *Server) routes() []route {
	routes := []route{
		{"/{$}", s.asset("index.html", "text/html; charset=utf-8")},
		{"/app.css", s.asset("app.css", "text/css; charset=utf-8")},
		{"/api/stats", s.handleStats},
		{"/api/fingerprint", s.handleFingerprint},
		{"/api/detail", s.handleDetail},
		{"/api/core", s.handleCore},
		{"/api/health", s.handleHealth},
		{"/api/graph", s.handleGraph},
		{"/api/graph/diff", s.handleGraphDiff},
		{"/api/events", s.handleEvents},
		{"/api/records", s.handleRecords},
		{"/api/record", s.handleRecord},
		{"/api/links", s.handleLinks},
		{"/api/history", s.handleHistory},
		{"/api/topics", s.handleTopics},
		{"/api/hygiene", s.handleHygiene},
		{"/api/search", s.handleSearch},
		{"/api/turns", s.handleTurns},
		{"/api/sessions", s.handleSessions},
	}
	return append(routes, s.scriptRoutes()...)
}

func Patterns() []string {
	var out []string
	for _, r := range (&Server{}).routes() {
		out = append(out, r.pattern)
	}
	return out
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, r := range s.routes() {
		mux.HandleFunc("GET "+r.pattern, r.handler)
	}
	return appHandler{Handler: s.guard(gzipMiddleware(mux)), s: s}
}

type appHandler struct {
	http.Handler
	s *Server
}

func (a appHandler) onShutdown() { a.s.ev.close() }

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", CSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if !s.allowRemote && !hostAllowed(r.Host) {
			writeError(w, http.StatusForbidden, "host not allowed")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "read-only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hostAllowed(hostport string) bool {
	host := hostport
	if h, port, err := net.SplitHostPort(hostport); err == nil {
		if !validPort(port) {
			return false
		}
		host = h
	}
	return isLoopbackHost(host)
}

func validPort(port string) bool {
	if port == "" {
		return true
	}
	_, err := strconv.ParseUint(port, 10, 16)
	return err == nil
}

func (s *Server) asset(name, ctype string) func(http.ResponseWriter, *http.Request) {
	data, err := fs.ReadFile(assetFS, "assets/"+name)
	return func(w http.ResponseWriter, _ *http.Request) {
		if err != nil {
			writeError(w, http.StatusInternalServerError, "asset missing")
			return
		}
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write(data)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) fingerprint(ctx context.Context) (string, error) {
	var parts [9]string
	err := s.st.DB().QueryRowContext(ctx, `SELECT
		(SELECT count(*) || '|' || coalesce(max(updated_at), '') || '|' || coalesce(max(verified_at), '') || '|' ||
			coalesce(sum(CASE status WHEN 'current' THEN 1 WHEN 'expired' THEN 2 WHEN 'retired' THEN 3 ELSE 5 END + core * 11), 0) FROM records),
		(SELECT count(*) FROM links), (SELECT count(*) FROM history), (SELECT count(*) FROM scopes), (SELECT count(*) FROM topics),
		(SELECT coalesce(group_concat(id || title), '') FROM topics), (SELECT count(*) FROM vectors), (SELECT count(*) FROM triggers),
		(SELECT coalesce(sum(record_id * (length(scope) * 131 + unicode(substr(scope, 7)) * 7 + unicode(substr(scope, -1)))), 0) FROM scopes)`).
		Scan(&parts[0], &parts[1], &parts[2], &parts[3], &parts[4], &parts[5], &parts[6], &parts[7], &parts[8])
	if err != nil {
		return "", err
	}
	return strings.Join(parts[:], "|"), nil
}

func (s *Server) Graph(ctx context.Context) (*Graph, error) {
	fp, err := s.fingerprint(ctx)
	if err != nil {
		return nil, err
	}
	fp += "|" + s.opt.now().Format(time.DateOnly)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.graph != nil && s.stamp == fp {
		return s.graph, nil
	}
	g, err := Build(ctx, s.st, s.opt)
	if err != nil {
		return nil, err
	}
	g.fp = fp
	s.graph, s.stamp = g, fp
	return g, nil
}

func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	if sh, ok := h.(interface{ onShutdown() }); ok {
		srv.RegisterOnShutdown(sh.onShutdown)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := srv.Shutdown(sctx)
		if serr := <-done; err == nil && !errors.Is(serr, http.ErrServerClosed) {
			err = serr
		}
		return err
	}
}

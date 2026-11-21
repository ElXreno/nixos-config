package ui

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

var gzipPool = sync.Pool{New: func() any {
	zw, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return zw
}}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		q := strings.TrimSpace(params)
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}

func compressible(ctype string) bool {
	ctype = strings.ToLower(ctype)
	for _, p := range []string{"application/json", "text/html", "text/css", "text/javascript", "text/plain", "image/svg+xml"} {
		if strings.HasPrefix(ctype, p) {
			return true
		}
	}
	return false
}

type gzipWriter struct {
	http.ResponseWriter
	zw      *gzip.Writer
	decided bool
	status  int
}

func (g *gzipWriter) decide() {
	if g.decided {
		return
	}
	g.decided = true
	h := g.Header()
	if h.Get("Content-Encoding") == "" && g.status != http.StatusNoContent && g.status != http.StatusNotModified && compressible(h.Get("Content-Type")) {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		g.zw = gzipPool.Get().(*gzip.Writer)
		g.zw.Reset(g.ResponseWriter)
	}
	if g.status != 0 {
		g.ResponseWriter.WriteHeader(g.status)
	}
}

func (g *gzipWriter) WriteHeader(status int) {
	if g.decided {
		return
	}
	g.status = status
	g.decide()
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	g.decide()
	if g.zw != nil {
		return g.zw.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipWriter) Flush() {
	g.decide()
	if g.zw != nil {
		_ = g.zw.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipWriter) finish() {
	if g.zw != nil {
		_ = g.zw.Close()
		g.zw.Reset(nil)
		gzipPool.Put(g.zw)
		g.zw = nil
	}
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		g := &gzipWriter{ResponseWriter: w}
		defer g.finish()
		next.ServeHTTP(g, r)
		g.decide()
	})
}

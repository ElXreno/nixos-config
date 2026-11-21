package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

func TestHostHeaderRejectsBadPorts(t *testing.T) {
	f := newFixture(t)
	for host, code := range map[string]int{
		"localhost:evil.com": http.StatusForbidden, "localhost:7781, evil.com": http.StatusForbidden, "127.0.0.1:99999": http.StatusForbidden,
		"127.0.0.1:+80": http.StatusForbidden, "[::1]:x": http.StatusForbidden,
		"localhost:7781": http.StatusOK, "[::1]:80": http.StatusOK, "[::1]": http.StatusOK, "LOCALHOST": http.StatusOK,
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		if rec.Code != code {
			t.Fatalf("Host %q = %d, want %d", host, rec.Code, code)
		}
	}
}

func TestGraphFollowsTheClock(t *testing.T) {
	clock := tNow
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	put(t, st, store.PutRequest{Key: "project_fresh", Kind: store.State, Title: sp("Fresh state"), Body: sp("Working on it.")})
	srv := New(st, Options{Now: func() time.Time { return clock }}, false)
	g1, err := srv.Graph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n := len(srv.hygieneOf(g1).Sections[3].Items); n != 0 {
		t.Fatalf("a new state is stale: %d", n)
	}
	clock = clock.Add(30 * 24 * time.Hour)
	g2, err := srv.Graph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st := srv.hygieneOf(g2).Sections[3]; st.ID != "stale" || len(st.Items) != 1 || st.Items[0].Key != "project_fresh" || g2.Built == g1.Built {
		t.Fatalf("after 30 days stale = %+v, built %s -> %s", st, g1.Built, g2.Built)
	}
}

func TestOnlyLogsAreChunked(t *testing.T) {
	f := newFixture(t)
	put(t, f.st, store.PutRequest{Key: "reference_numbered#1", Kind: store.Fact, Title: sp("Numbered fact"), Body: sp("Not a log chunk.")})
	if rec := f.do(t, http.MethodGet, "/api/record?key=reference_numbered"); rec.Code != http.StatusNotFound {
		t.Fatalf("a fact with a numbered key assembled as a chunked record: %d", rec.Code)
	}
	var out struct {
		Versions []store.Version `json:"versions"`
	}
	put(t, f.st, store.PutRequest{Key: "reference_numbered#1", Kind: store.Fact, Title: sp("Numbered fact"), Body: sp("Revised."), Update: true})
	f.json(t, "/api/history?key=reference_numbered", &out)
	if len(out.Versions) != 0 {
		t.Fatalf("history of a non-existent key returned %d versions of a fact", len(out.Versions))
	}
	var rec struct {
		Record store.Record `json:"record"`
	}
	f.json(t, "/api/record?key=project_big", &rec)
	if rec.Record.Key != "project_big" || !strings.Contains(rec.Record.Body, "nvidia") {
		t.Fatalf("chunked log no longer assembles: %+v", rec.Record.Key)
	}
}

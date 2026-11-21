package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestReindexDefaultsToTheDocumentPrefix(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var mu sync.Mutex
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		sent = append(sent, req.Input...)
		mu.Unlock()
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i, in := range req.Input {
			data = append(data, item{i, axisVec(in)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	if code, out, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis"); code != 0 {
		t.Fatalf("reindex: %d %q %q", code, out, errs)
	}
	if len(sent) != 3 {
		t.Fatalf("%d texts sent", len(sent))
	}
	for _, s := range sent {
		if !strings.HasPrefix(s, "Document: ") {
			t.Errorf("text sent without the document prefix: %.20q", s)
		}
	}
	code, out, _ := sb.run("", "vec", "status", "--model", "axis")
	if code != 0 || !strings.Contains(out, "fresh 3 stale 0 missing 0") {
		t.Errorf("status with the default prefix: %d %q", code, out)
	}
	sent = nil
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "", "--force"); code != 0 {
		t.Fatalf("reindex without a prefix: %d %q", code, errs)
	}
	for _, s := range sent {
		if strings.HasPrefix(s, "Document: ") {
			t.Errorf("an explicit empty prefix was replaced: %.20q", s)
		}
	}
}

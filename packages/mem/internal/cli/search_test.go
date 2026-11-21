package cli

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mem/internal/store"
)

func TestSearchDefaultIsHybridWithRerank(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs, rr atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: "); code != 0 {
		t.Fatalf("%s", errs)
	}
	var docs atomic.Value
	rsrv := rerankStub(t, map[string]float64{"half of RAM": 0.9987, "explicit authorization": 0.25}, &rr, &docs)

	before := reqs.Load()
	code, out, errs := sb.run("", "search", "upstream")
	if code != 0 || out != "no matches\n" || errs != "" || reqs.Load() != before {
		t.Fatalf("no embedder configured: %d %q %q, %d embed requests", code, out, errs, reqs.Load()-before)
	}

	sb.vars["MEM_EMBED_URL"] = srv.URL
	code, out, errs = sb.run("", "search", "upstream")
	if code != 0 || out != "no matches\n" || !strings.Contains(errs, "an embedder URL without a model") {
		t.Fatalf("url without a model: %d %q %q", code, out, errs)
	}

	sb.vars["MEM_EMBED_MODEL"] = "axis"
	code, out, errs = sb.run("", "search", "upstream")
	if code != 0 || !strings.HasPrefix(out, "1. no-push-default") || errs != "" || reqs.Load() != before+1 || rr.Load() != 0 {
		t.Fatalf("hybrid without a reranker: %d %q %q, %d embed and %d rerank requests", code, out, errs, reqs.Load()-before, rr.Load())
	}

	sb.vars["MEM_RERANK_URL"] = rsrv.URL
	code, out, errs = sb.run("", "search", "commits", "ARC")
	lines := strings.Split(out, "\n")
	if code != 0 || errs != "" || rr.Load() != 1 || !strings.HasPrefix(lines[0], "1. zfs-arc-ram") || !strings.Contains(lines[0], "rerank 0.9987") || !strings.Contains(out, "2. no-push-default") {
		t.Fatalf("hybrid with rerank: %d %q, %d rerank requests\n%s", code, errs, rr.Load(), out)
	}

	code, out, _ = sb.run("", "search", "--no-rerank", "commits", "ARC")
	if code != 0 || rr.Load() != 1 || strings.Contains(out, "rerank ") {
		t.Fatalf("--no-rerank: %d, %d rerank requests\n%s", code, rr.Load(), out)
	}

	embeds := reqs.Load()
	for _, mode := range []string{"fused", "words", "trigram"} {
		if code, _, _ := sb.run("", "search", "--mode", mode, "ARC"); code != 0 || reqs.Load() != embeds || rr.Load() != 1 {
			t.Errorf("--mode %s reached a service: %d embed, %d rerank requests", mode, reqs.Load()-embeds, rr.Load()-1)
		}
	}

	code, out, _ = sb.run("", "search", "bluetooth", "headset")
	if code != 0 || strings.Contains(out, "bt-journal") {
		t.Errorf("a log record without --logs: %d %q", code, out)
	}
	code, out, _ = sb.run("", "search", "--logs", "bluetooth", "headset")
	if code != 0 || !strings.Contains(out, "bt-journal#1 [log") {
		t.Errorf("--logs: %d %q", code, out)
	}
	for _, args := range [][]string{{"--all-chunks"}, {"--kind", "log"}, {"--mode", "fused", "--logs"}} {
		if code, out, _ := sb.run("", append(append([]string{"search"}, args...), "bluetooth", "headset")...); code != 0 || !strings.Contains(out, "bt-journal#1") {
			t.Errorf("%v: %d %q", args, code, out)
		}
	}
	if code, out, _ := sb.run("", "search", "--mode", "fused", "bluetooth", "headset"); code != 0 || strings.Contains(out, "bt-journal") {
		t.Errorf("--mode fused lists a log record without --logs: %d %q", code, out)
	}
	if code, _, errs := sb.run("", "search", "--rerank", "--no-rerank", "x"); code != 2 || !strings.Contains(errs, "exclude each other") {
		t.Errorf("--rerank --no-rerank: %d %q", code, errs)
	}
}

func TestSearchAllChunksIsLexicalOnly(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	if code, _, errs := sb.run("2026-04-13 bluetooth headset pairing worked after a firmware update.", "put", "--key", "bt-journal#2", "--kind", "log", "--scope", "global", "--distinct", "bt-journal#1", "--why", "next day"); code != 0 {
		t.Fatalf("put: %s", errs)
	}
	var reqs, rr atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: "); code != 0 {
		t.Fatalf("%s", errs)
	}
	var docs atomic.Value
	rsrv := rerankStub(t, map[string]float64{"bluetooth": 0.5}, &rr, &docs)
	sb.vars["MEM_EMBED_URL"], sb.vars["MEM_EMBED_MODEL"], sb.vars["MEM_RERANK_URL"] = srv.URL, "axis", rsrv.URL
	embeds := reqs.Load()
	for _, args := range [][]string{{"--all-chunks"}, {"--mode", "fused", "--all-chunks"}} {
		code, out, errs := sb.run("", append(append([]string{"search"}, args...), "bluetooth", "headset")...)
		if code != 0 || errs != "" || !strings.Contains(out, "bt-journal#1") || !strings.Contains(out, "bt-journal#2") || reqs.Load() != embeds || rr.Load() != 0 {
			t.Errorf("%v: %d stdout %q stderr %q, %d embed and %d rerank requests", args, code, out, errs, reqs.Load()-embeds, rr.Load())
		}
	}
	for _, mode := range []string{"vector", "hybrid"} {
		code, out, errs := sb.run("", "search", "--mode", mode, "--all-chunks", "bluetooth", "headset")
		if code != 2 || out != "" || !strings.Contains(errs, "--all-chunks") || !strings.Contains(errs, "fused, words or trigram") || reqs.Load() != embeds {
			t.Errorf("--mode %s --all-chunks: %d stdout %q stderr %q", mode, code, out, errs)
		}
	}
}

func TestSearchDefaultFallsBackToLexical(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	var reqs, rr atomic.Int32
	srv := axisServer(t, &reqs)
	if code, _, errs := sb.run("", "reindex", "--url", srv.URL, "--model", "axis", "--doc-prefix", "Document: "); code != 0 {
		t.Fatalf("%s", errs)
	}
	var docs atomic.Value
	rsrv := rerankStub(t, map[string]float64{"half of RAM": 0.9}, &rr, &docs)

	sb.vars["MEM_EMBED_URL"], sb.vars["MEM_EMBED_MODEL"], sb.vars["MEM_RERANK_URL"] = "http://127.0.0.1:1", "axis", rsrv.URL
	start := time.Now()
	code, out, errs := sb.run("", "search", "ARC")
	if code != 0 || !strings.HasPrefix(out, "1. zfs-arc-ram") || errs != "mem search: embedder down, lexical results only\n" || rr.Load() != 0 {
		t.Fatalf("embedder down: %d %q %q, %d rerank requests", code, out, errs, rr.Load())
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("fallback took %s", took)
	}
	code, out, errs = sb.run("", "search", "--json", "ARC")
	var hits []struct{ Record struct{ Key string } }
	if code != 0 || json.Unmarshal([]byte(out), &hits) != nil || len(hits) != 1 || hits[0].Record.Key != "zfs-arc-ram" || !strings.Contains(errs, "lexical results only") {
		t.Errorf("--json keeps stdout clean: %d %q %q", code, out, errs)
	}

	sb.vars["MEM_EMBED_MODEL"] = "other-model"
	sb.vars["MEM_EMBED_URL"] = srv.URL
	code, out, errs = sb.run("", "search", "ARC")
	if code != 0 || !strings.HasPrefix(out, "1. zfs-arc-ram") || !strings.Contains(errs, "embedder has no stored vectors for this model, lexical results only") {
		t.Errorf("no vectors for the model: %d %q %q", code, out, errs)
	}

	sb.vars["MEM_EMBED_MODEL"], sb.vars["MEM_RERANK_URL"] = "axis", "http://127.0.0.1:1"
	code, out, errs = sb.run("", "search", "ARC")
	if code != 0 || !strings.HasPrefix(out, "1. zfs-arc-ram") || strings.Contains(out, "rerank ") || errs != "mem search: reranker down, order left as fused\n" {
		t.Errorf("reranker down: %d %q %q", code, out, errs)
	}

	code, _, errs = sb.run("", "search", "--mode", "hybrid", "--embed-url", "http://127.0.0.1:1", "ARC")
	if code != 1 || !strings.Contains(errs, "embedding the query (down)") {
		t.Errorf("an explicit mode still fails: %d %q", code, errs)
	}
}

func TestSearchMarksExpiredState(t *testing.T) {
	sb := newSandbox(t)
	if code, _, errs := sb.run("", "put", "relay-state", "--kind", "state", "--scope", "global", "--title", "Relay board state", "--ttl", "1",
		"--body", "The relay board firmware is at revision 7 and the watchdog is disabled."); code != 0 {
		t.Fatalf("put: %s", errs)
	}
	put(t, sb, "relay-fact", "The relay board has eight channels.")
	sb.now = sb.now.Add(72 * time.Hour)
	if code, out, errs := sb.run("", "expire"); code != 0 || !strings.Contains(out, "relay-state\texpired") {
		t.Fatalf("expire: %d %q %q", code, out, errs)
	}
	code, out, _ := sb.run("", "search", "relay", "board", "firmware", "watchdog")
	if code != 0 || !strings.Contains(out, "1. relay-state [state, EXPIRED since 2026-11-10]") ||
		!strings.Contains(out, "expired state: check it, then `mem verify relay-state` if it still holds, `mem retire relay-state --why TEXT` if not") {
		t.Fatalf("expired state in search:\n%s", out)
	}
	if strings.Count(out, "expired state:") != 1 {
		t.Errorf("the hint belongs to the expired record only:\n%s", out)
	}
	code, out, _ = sb.run("", "search", "--json", "relay", "firmware")
	var hits []struct{ Record store.Record }
	if code != 0 || json.Unmarshal([]byte(out), &hits) != nil || len(hits) == 0 || hits[0].Record.Status != store.Expired {
		t.Fatalf("json status: %d %s", code, out)
	}
	if code, _, errs := sb.run("", "retire", "relay-state", "--why", "board replaced"); code != 0 {
		t.Fatalf("retire: %s", errs)
	}
	code, out, _ = sb.run("", "search", "relay", "board", "firmware", "watchdog")
	if code != 0 || strings.Contains(out, "relay-state") {
		t.Errorf("a retired record without --all-status:\n%s", out)
	}
	code, out, _ = sb.run("", "search", "--all-status", "relay", "board", "firmware", "watchdog")
	if code != 0 || !strings.Contains(out, "relay-state [state, retired") {
		t.Errorf("--all-status:\n%s", out)
	}
}

func TestReindexEmbedsExpiredState(t *testing.T) {
	sb := newSandbox(t)
	seedForVectors(t, sb)
	if code, _, errs := sb.run("", "put", "arc-state", "--kind", "state", "--scope", "global", "--title", "ARC limit state", "--ttl", "1",
		"--body", "The zfs ARC limit on the tablet is 6 GiB as of today."); code != 0 {
		t.Fatalf("put: %s", errs)
	}
	sb.now = sb.now.Add(72 * time.Hour)
	if code, out, errs := sb.run("", "expire"); code != 0 || !strings.Contains(out, "arc-state\texpired") {
		t.Fatalf("expire: %d %q %q", code, out, errs)
	}
	var reqs atomic.Int32
	srv := axisServer(t, &reqs)
	sb.vars["MEM_EMBED_URL"], sb.vars["MEM_EMBED_MODEL"] = srv.URL, "axis"
	code, out, _ := sb.run("", "vec", "status")
	if code != 0 || !strings.Contains(out, "records 4 current 3 expired 1") || !strings.Contains(out, "fresh 0 stale 0 missing 4") {
		t.Fatalf("status before: %d %q", code, out)
	}
	code, out, errs := sb.run("", "reindex")
	if code != 0 || !strings.Contains(out, "current 3, expired 1, fresh 0, embedded 4") {
		t.Fatalf("reindex: %d %q %q", code, out, errs)
	}
	code, out, _ = sb.run("", "vec", "status")
	if code != 0 || !strings.Contains(out, "fresh 4 stale 0 missing 0") {
		t.Fatalf("status after: %d %q", code, out)
	}
	code, out, errs = sb.run("", "search", "--no-rerank", "memory", "pressure", "ARC")
	if code != 0 || errs != "" || !strings.Contains(out, "arc-state [state, EXPIRED since 2026-11-10]") {
		t.Fatalf("hybrid search lists the expired record: %d %q %q", code, out, errs)
	}
}

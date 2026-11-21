package recall

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mem/internal/store"
)

type fakeEmbedder struct {
	mu      sync.Mutex
	dim     int
	texts   []string
	calls   int
	onCall  func(n int)
	failAt  int
	nan     bool
	dropOne bool
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.texts = append(f.texts, texts...)
	f.mu.Unlock()
	if f.onCall != nil {
		f.onCall(n)
	}
	if f.failAt != 0 && n == f.failAt {
		return nil, errors.New("embedder down")
	}
	dim := f.dim
	if dim == 0 {
		dim = 8
	}
	var out [][]float32
	for _, t := range texts {
		sum := sha256.Sum256([]byte(t))
		v := make([]float32, dim)
		var s float64
		for i := range v {
			v[i] = float32(int(sum[i%32])-120) / 64
			s += float64(v[i]) * float64(v[i])
		}
		for i := range v {
			v[i] = float32(float64(v[i]) / math.Sqrt(s))
		}
		if f.nan {
			v[0] = float32(math.NaN())
		}
		out = append(out, v)
	}
	if f.dropOne {
		out = out[:len(out)-1]
	}
	return out, nil
}

func reindexStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	recs := []store.Record{
		{Key: "rule-a", Kind: store.Rule, Scopes: []string{"global"}, Title: "Rule A", Body: "never push by default"},
		{Key: "fact-b", Kind: store.Fact, Scopes: []string{"global"}, Title: "Fact B", Body: "arc grows to half of ram"},
		{Key: "journal#1", Kind: store.Log, Scopes: []string{"global"}, Body: "a log chunk"},
		{Key: "old-c", Kind: store.Rule, Scopes: []string{"global"}, Status: store.Superseded, Title: "Old", Body: "retired text"},
	}
	for i := range 6 {
		recs = append(recs, store.Record{Key: fmt.Sprintf("extra-%d", i), Kind: store.Fact, Scopes: []string{"global"}, Title: fmt.Sprintf("Extra %d", i), Body: strings.Repeat("filler ", i+1)})
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	return s
}

func opts(e Embedder) ReindexOptions {
	return ReindexOptions{Model: "m", DocPrefix: "Document: ", Embedder: e, Batch: 2, Par: 3}
}

func TestReindexEmbedsMissingThenNothing(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	e := &fakeEmbedder{}
	res, err := Reindex(ctx, s, opts(e))
	if err != nil {
		t.Fatal(err)
	}
	if res.Current != 9 || res.Embedded != 9 || res.Fresh != 0 {
		t.Errorf("first run %+v", res)
	}
	for _, text := range e.texts {
		if !strings.HasPrefix(text, "Document: ") {
			t.Errorf("text sent without the prefix: %q", text[:min(len(text), 20)])
		}
	}
	info, _ := s.VectorInfo(ctx, "m")
	if _, ok := info["old-c"]; ok || len(info) != 9 {
		t.Errorf("%d vectors, superseded one stored: %v", len(info), ok)
	}
	e2 := &fakeEmbedder{}
	res, err = Reindex(ctx, s, opts(e2))
	if err != nil || res.Embedded != 0 || res.Fresh != 9 || e2.calls != 0 {
		t.Errorf("second run %+v, %d calls, %v", res, e2.calls, err)
	}
}

func TestReindexCountsExpiredApartFromCurrent(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	exp := store.Record{Key: "gone-d", Kind: store.State, Scopes: []string{"global"}, Status: store.Expired, Title: "Gone", Body: "expired state", ValidTo: at}
	if _, err := s.Upsert(ctx, []store.Record{exp}, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	res, err := Reindex(ctx, s, opts(&fakeEmbedder{}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Current != 9 || res.Expired != 1 || res.Embedded != 10 {
		t.Errorf("want current 9, expired 1, embedded 10: %+v", res)
	}
	res, err = Reindex(ctx, s, ReindexOptions{Model: "m", DocPrefix: "Document: ", Embedder: &fakeEmbedder{}, SkipLogs: true})
	if err != nil || res.Current != 8 || res.Expired != 1 || res.Fresh != 9 || res.SkippedLogs != 1 {
		t.Errorf("second run without logs: want current 8, expired 1, fresh 9, skipped logs 1: %+v %v", res, err)
	}
}

func TestReindexStaleCases(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	if _, err := Reindex(ctx, s, opts(&fakeEmbedder{})); err != nil {
		t.Fatal(err)
	}
	body := "never push by default, and never force"
	if _, err := s.Put(ctx, store.PutRequest{Key: "rule-a", Kind: store.Rule, Scopes: []string{"global"}, Update: true, Body: &body, Gate: store.DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	e := &fakeEmbedder{}
	res, err := Reindex(ctx, s, opts(e))
	if err != nil || res.Embedded != 1 || res.Fresh != 8 || len(e.texts) != 1 || !strings.Contains(e.texts[0], "never force") {
		t.Errorf("edited text: %+v %v", res, err)
	}
	o := opts(&fakeEmbedder{})
	o.DocPrefix = "Passage: "
	res, err = Reindex(ctx, s, o)
	if err != nil || res.Embedded != 9 {
		t.Errorf("new prefix: %+v %v", res, err)
	}
	o.Force = true
	res, err = Reindex(ctx, s, o)
	if err != nil || res.Embedded != 9 || res.Fresh != 0 {
		t.Errorf("force: %+v %v", res, err)
	}
	o.Model = "other"
	res, err = Reindex(ctx, s, o)
	if err != nil || res.Embedded != 9 {
		t.Errorf("other model: %+v %v", res, err)
	}
	if models, _ := s.VectorModels(ctx); len(models) != 2 {
		t.Errorf("models %+v", models)
	}
}

func TestReindexSkipLogs(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	o := opts(&fakeEmbedder{})
	o.SkipLogs = true
	res, err := Reindex(ctx, s, o)
	if err != nil || res.Embedded != 8 || res.SkippedLogs != 1 {
		t.Errorf("%+v %v", res, err)
	}
	if info, _ := s.VectorInfo(ctx, "m"); len(info) != 8 {
		t.Errorf("%d vectors", len(info))
	}
}

func TestReindexFailuresKeepProgress(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		e    *fakeEmbedder
		want string
	}{
		{"server error", &fakeEmbedder{failAt: 3}, "embedder down"},
		{"NaN vector", &fakeEmbedder{nan: true}, "non-finite"},
		{"missing vector", &fakeEmbedder{dropOne: true}, "vectors for"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := reindexStore(t)
			o := opts(c.e)
			o.Par = 1
			_, err := Reindex(ctx, s, o)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%v, want %q", err, c.want)
			}
			info, _ := s.VectorInfo(ctx, "m")
			if c.name == "server error" && len(info) != 4 {
				t.Errorf("progress before the failure: %d vectors, want 4", len(info))
			}
			if c.name != "server error" && len(info) != 0 {
				t.Errorf("%d vectors stored from a bad response", len(info))
			}
		})
	}
}

func TestReindexDimensionChange(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	if _, err := Reindex(ctx, s, opts(&fakeEmbedder{dim: 8})); err != nil {
		t.Fatal(err)
	}
	body := "changed"
	if _, err := s.Put(ctx, store.PutRequest{Key: "fact-b", Kind: store.Fact, Scopes: []string{"global"}, Update: true, Body: &body, Gate: store.DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	_, err := Reindex(ctx, s, opts(&fakeEmbedder{dim: 16}))
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("%v", err)
	}
	o := opts(&fakeEmbedder{dim: 16})
	o.Force = true
	if res, err := Reindex(ctx, s, o); err != nil || res.Embedded != 9 {
		t.Errorf("force: %+v %v", res, err)
	}
}

func TestReindexRacesAnEdit(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	e := &fakeEmbedder{}
	once := sync.Once{}
	e.onCall = func(int) {
		once.Do(func() {
			body := "edited while the batch was being embedded"
			for _, k := range []string{"rule-a", "fact-b", "journal#1"} {
				rec, _ := s.Get(ctx, k)
				if _, err := s.Put(ctx, store.PutRequest{Key: k, Kind: rec.Kind, Scopes: rec.Scopes, Update: true, Body: &body, Gate: store.DefaultGate()}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	o := opts(e)
	o.Par = 1
	o.Batch = 9
	res, err := Reindex(ctx, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if res.Raced != 3 || res.Embedded != 6 {
		t.Errorf("%+v", res)
	}
	res, err = Reindex(ctx, s, opts(&fakeEmbedder{}))
	if err != nil || res.Embedded != 3 {
		t.Errorf("next run %+v %v", res, err)
	}
}

func TestReindexMaxDocsTakesTheShortestFirst(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	e := &fakeEmbedder{}
	o := opts(e)
	o.MaxDocs = 4
	res, err := Reindex(ctx, s, o)
	if err != nil || res.Embedded != 4 || res.Deferred != 5 {
		t.Fatalf("%+v %v", res, err)
	}
	longest := 0
	for _, text := range e.texts {
		longest = max(longest, len(text))
	}
	e2 := &fakeEmbedder{}
	o.Embedder = e2
	res, err = Reindex(ctx, s, o)
	if err != nil || res.Embedded != 4 || res.Deferred != 1 || res.Fresh != 4 {
		t.Fatalf("second run %+v %v", res, err)
	}
	for _, text := range e2.texts {
		if len(text) < longest {
			t.Errorf("a shorter text (%d) came after a longer one (%d)", len(text), longest)
		}
	}
	o.Embedder = &fakeEmbedder{}
	if res, err = Reindex(ctx, s, o); err != nil || res.Embedded != 1 || res.Deferred != 0 {
		t.Errorf("third run %+v %v", res, err)
	}
}

func TestReindexIsIncrementalForATimer(t *testing.T) {
	ctx := context.Background()
	s := reindexStore(t)
	if _, err := Reindex(ctx, s, opts(&fakeEmbedder{})); err != nil {
		t.Fatal(err)
	}
	down := &fakeEmbedder{failAt: 1}
	res, err := Reindex(ctx, s, opts(down))
	if err != nil || down.calls != 0 || res.Embedded != 0 || res.Fresh != 9 {
		t.Fatalf("nothing to do must not touch the embedder: %+v, %d calls, %v", res, down.calls, err)
	}
	if _, err := s.SetTopics(ctx, []store.TopicAssign{{Key: "rule-a", Topic: "git", Title: "Git"}, {Key: "fact-b", Topic: "hw", Title: "Hardware"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, "fact-b", "checked"); err != nil {
		t.Fatal(err)
	}
	core := true
	if _, err := s.Put(ctx, store.PutRequest{Key: "extra-0", Update: true, Core: &core, Gate: store.DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retire(ctx, "extra-1", "gone"); err != nil {
		t.Fatal(err)
	}
	e := &fakeEmbedder{}
	res, err = Reindex(ctx, s, opts(e))
	if err != nil || e.calls != 0 || res.Embedded != 0 || res.Fresh != 8 || res.Current != 8 {
		t.Fatalf("topic, verify, core and retire changes must keep the vectors: %+v, %d calls, %v", res, e.calls, err)
	}
	body := "a freshly written record about reindex timers"
	if _, err := s.Put(ctx, store.PutRequest{Key: "fresh-one", Kind: store.Fact, Scopes: []string{"global"}, Title: ptr("Fresh"), Body: &body, Gate: store.DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	e = &fakeEmbedder{}
	res, err = Reindex(ctx, s, opts(e))
	if err != nil || res.Embedded != 1 || res.Fresh != 8 || len(e.texts) != 1 || !strings.Contains(e.texts[0], "reindex timers") {
		t.Fatalf("a new record is the only one embedded: %+v, %v", res, err)
	}
}

func ptr[T any](v T) *T { return &v }

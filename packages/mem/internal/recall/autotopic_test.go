package recall

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mem/internal/store"
)

func autoTopicStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, ctx
}

type autoRec struct {
	key, project, topic string
	vec                 []float32
}

func seedAuto(t *testing.T, s *store.Store, ctx context.Context, recs []autoRec) {
	t.Helper()
	var rs []store.Record
	for _, r := range recs {
		rs = append(rs, store.Record{Key: r.key, Kind: store.Fact, Scopes: []string{"project:" + r.project}, Title: "T " + r.key, Body: "body of " + r.key})
	}
	if _, err := s.Upsert(ctx, rs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	var as []store.TopicAssign
	for _, r := range recs {
		if r.topic != "" {
			as = append(as, store.TopicAssign{Key: r.key, Topic: r.topic, Title: "Topic " + r.topic})
		}
	}
	if _, err := s.SetTopics(ctx, as); err != nil {
		t.Fatal(err)
	}
	var rows []store.VecRow
	for _, r := range recs {
		got, err := s.Get(ctx, r.key)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, store.VecRow{Key: r.key, Vec: r.vec, TextSHA: store.TextSHA(store.DocText("", got))})
	}
	if _, _, err := s.PutVectors(ctx, "m", "", rows); err != nil {
		t.Fatal(err)
	}
}

func topicOf(t *testing.T, s *store.Store, ctx context.Context, key string) string {
	t.Helper()
	r, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	return r.Topic()
}

func TestAutoTopicsVotesNearestAndSkipsFarOnes(t *testing.T) {
	s, ctx := autoTopicStore(t)
	seedAuto(t, s, ctx, []autoRec{
		{"a1", "p", "alpha", []float32{1, 0, 0}},
		{"a2", "p", "alpha", []float32{.9, .1, 0}},
		{"a3", "p", "alpha", []float32{.95, 0, .05}},
		{"b1", "p", "beta", []float32{0, 1, 0}},
		{"b2", "p", "beta", []float32{.1, .9, 0}},
		{"new-near-alpha", "p", "", []float32{.97, .05, 0}},
		{"new-far", "p", "", []float32{0, 0, 1}},
	})
	res, err := AutoTopics(ctx, s, AutoTopicOptions{Model: "m", K: 3, MinCos: .45})
	if err != nil {
		t.Fatal(err)
	}
	if got := topicOf(t, s, ctx, "new-near-alpha"); got != "alpha" {
		t.Errorf("new-near-alpha got topic %q, want alpha", got)
	}
	if got := topicOf(t, s, ctx, "new-far"); got != "" {
		t.Errorf("new-far got topic %q, want none below the cosine floor", got)
	}
	if len(res.Assigned) != 1 || res.Skipped != 1 {
		t.Errorf("assigned %v skipped %d, want one assignment and one skip", res.Assigned, res.Skipped)
	}
	if topicOf(t, s, ctx, "a1") != "alpha" || topicOf(t, s, ctx, "b1") != "beta" {
		t.Error("existing topics changed")
	}
	again, err := AutoTopics(ctx, s, AutoTopicOptions{Model: "m", K: 3, MinCos: .45})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Assigned) != 0 {
		t.Errorf("second run assigned %v, want nothing new", again.Assigned)
	}
}

func TestAutoTopicsPrefersTheRecordsOwnProject(t *testing.T) {
	s, ctx := autoTopicStore(t)
	seedAuto(t, s, ctx, []autoRec{
		{"o1", "other", "foreign", []float32{1, 0}},
		{"o2", "other", "foreign", []float32{1, 0}},
		{"o3", "other", "foreign", []float32{1, 0}},
		{"m1", "mine", "home", []float32{.8, .6}},
		{"m2", "mine", "home", []float32{.8, .6}},
		{"m3", "mine", "home", []float32{.8, .6}},
		{"fresh", "mine", "", []float32{1, 0}},
	})
	if _, err := AutoTopics(ctx, s, AutoTopicOptions{Model: "m", K: 3, MinCos: .45}); err != nil {
		t.Fatal(err)
	}
	if got := topicOf(t, s, ctx, "fresh"); got != "home" {
		t.Errorf("fresh got topic %q, want home from its own project", got)
	}
}

func TestAutoTopicsDoNotVoteForLaterOnes(t *testing.T) {
	s, ctx := autoTopicStore(t)
	seedAuto(t, s, ctx, []autoRec{
		{"o1", "other", "right", []float32{1, 0}},
		{"o2", "other", "right", []float32{1, 0}},
		{"o3", "other", "right", []float32{1, 0}},
		{"m1", "mine", "sink", []float32{0, 1}},
		{"m2", "mine", "", []float32{.1, .99}},
		{"m2b", "mine", "", []float32{.1, .99}},
		{"m2c", "mine", "", []float32{.1, .99}},
	})
	if _, err := AutoTopics(ctx, s, AutoTopicOptions{Model: "m", K: 3, MinCos: .45}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"m2", "m2b", "m2c"} {
		if got := topicOf(t, s, ctx, k); got != "sink" {
			t.Fatalf("%s got topic %q, want sink", k, got)
		}
	}
	for _, k := range []string{"m3", "m4", "m5"} {
		if _, err := s.Upsert(ctx, []store.Record{{Key: k, Kind: store.Fact, Scopes: []string{"project:mine"}, Title: "T " + k, Body: "body of " + k}}, nil, "seed"); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.PutVectors(ctx, "m", "", []store.VecRow{{Key: k, Vec: []float32{.8, .6}, TextSHA: store.TextSHA(store.DocText("", got))}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AutoTopics(ctx, s, AutoTopicOptions{Model: "m", K: 3, MinCos: .45}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"m3", "m4", "m5"} {
		if got := topicOf(t, s, ctx, k); got != "right" {
			t.Errorf("%s got topic %q, want right: the automatic sink topic of m2 must not vote", k, got)
		}
	}
	if _, err := s.SetTopics(ctx, []store.TopicAssign{{Key: "m2", Topic: "sink", Title: "Topic sink"}}); err != nil {
		t.Fatal(err)
	}
	auto, err := s.AutoTopicKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if auto["m2"] || !auto["m3"] {
		t.Errorf("auto keys %v: a manual set must clear the mark, automatic ones keep it", auto)
	}
}

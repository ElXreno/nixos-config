package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func setUpdatedAt(t *testing.T, s *Store, key string, at time.Time) {
	t.Helper()
	err := s.tx(context.Background(), func(tx *sql.Tx) error {
		rw, err := s.getRow(context.Background(), tx, key)
		if err != nil {
			return err
		}
		rw.rec.UpdatedAt = at
		return updateRecord(context.Background(), tx, rw.id, rw.rec)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func triggers(regexes ...string) *[]Trigger {
	out := make([]Trigger, len(regexes))
	for i, re := range regexes {
		out[i] = Trigger{Tool: "Bash", Regex: re}
	}
	return &out
}

func TestTriggerOnlyUpdateKeepsAsOf(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	born := c.t
	mustPut(t, s, spec{key: "nix-build-rule", title: "Build rule", body: "Build before deploy."}.req())

	c.advance(48 * time.Hour)
	res := mustPut(t, s, PutRequest{Key: "nix-build-rule", Update: true, Triggers: triggers(`nix\s+build`), Gate: DefaultGate()})
	if res.Version != 2 || res.Unchanged {
		t.Fatalf("trigger update = %+v", res)
	}
	got, err := s.Get(ctx, "nix-build-rule")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.Equal(born) || !got.AsOf().Equal(born) || len(got.Triggers) != 1 {
		t.Errorf("after a trigger-only update: updated %v, as of %v, triggers %v; want as of %v", got.UpdatedAt, got.AsOf(), got.Triggers, born)
	}
	h, err := s.History(ctx, "nix-build-rule")
	if err != nil || len(h) != 1 || h[0].Version != 1 {
		t.Fatalf("history = %+v, %v", h, err)
	}

	c.advance(time.Hour)
	mustPut(t, s, PutRequest{Key: "nix-build-rule", Update: true, Body: ptr("Build before deploy, only when code changed."), Gate: DefaultGate()})
	got, err = s.Get(ctx, "nix-build-rule")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.Equal(c.t) {
		t.Errorf("after a body update: updated %v, want %v", got.UpdatedAt, c.t)
	}
}

func TestRetime(t *testing.T) {
	ctx := context.Background()
	s, c := newStore(t)
	born := c.t
	mustPut(t, s, spec{key: "chained", title: "Chained", body: "Two trigger passes."}.req())
	mustPut(t, s, spec{key: "edited", title: "Edited", body: "Old text."}.req())
	mustPut(t, s, spec{key: "untouched", title: "Untouched", body: "Never updated."}.req())

	c.advance(24 * time.Hour)
	first := c.t
	mustPut(t, s, PutRequest{Key: "chained", Update: true, Triggers: triggers("one"), Gate: DefaultGate()})
	setUpdatedAt(t, s, "chained", first)

	c.advance(24 * time.Hour)
	second := c.t
	mustPut(t, s, PutRequest{Key: "chained", Update: true, Triggers: triggers("one", "two"), Gate: DefaultGate()})
	setUpdatedAt(t, s, "chained", second)
	mustPut(t, s, PutRequest{Key: "edited", Update: true, Body: ptr("New text."), Triggers: triggers("x"), Gate: DefaultGate()})

	listed, err := s.Retime(ctx, second, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Key != "chained" || !listed[0].From.Equal(second) || !listed[0].To.Equal(first) {
		t.Fatalf("since the second pass = %+v, want chained back to %v", listed, first)
	}
	if got, _ := s.Get(ctx, "chained"); !got.UpdatedAt.Equal(second) {
		t.Fatalf("a listing changed the record: %v", got.UpdatedAt)
	}

	done, err := s.Retime(ctx, time.Time{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || done[0].Key != "chained" || !done[0].To.Equal(born) {
		t.Fatalf("applied = %+v, want chained back to %v", done, born)
	}
	got, err := s.Get(ctx, "chained")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.Equal(born) || got.Version != 3 || len(got.Triggers) != 2 {
		t.Errorf("chained = updated %v version %d triggers %v", got.UpdatedAt, got.Version, got.Triggers)
	}
	if e, _ := s.Get(ctx, "edited"); !e.UpdatedAt.Equal(second) {
		t.Errorf("edited moved to %v, want %v", e.UpdatedAt, second)
	}
	again, err := s.Retime(ctx, time.Time{}, true)
	if err != nil || len(again) != 0 {
		t.Errorf("second run = %+v, %v", again, err)
	}
}

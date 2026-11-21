package store

import (
	"context"
	"errors"
	"testing"
)

func TestEditCountsOverlappingOccurrencesAsAmbiguous(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	for _, c := range []struct {
		key, body, old string
		count          int
	}{
		{"overlap_space", "Ports on the stand are x x x for the probe.", "x x", 2},
		{"overlap_run", "The probe log shows aaa before the reset.", "aa", 2},
		{"overlap_rune", "Пороги ааа на стенде.", "аа", 2},
	} {
		mustPut(t, s, spec{key: c.key, title: "Overlap " + c.key, body: c.body}.req())
		_, err := s.Edit(ctx, EditRequest{Key: c.key, Old: c.old, New: "y"})
		var me *MatchError
		if !errors.As(err, &me) || me.Count != c.count {
			rec, gerr := s.Get(ctx, c.key)
			if gerr != nil {
				t.Fatal(gerr)
			}
			t.Errorf("%s: --old %q in %q: err %v, body now %q, want a MatchError with count %d", c.key, c.old, c.body, err, rec.Body, c.count)
		}
	}
	if _, err := s.Edit(ctx, EditRequest{Key: "overlap_space", Old: "x x", New: "y", All: true}); err != nil {
		t.Fatal(err)
	}
	if rec, err := s.Get(ctx, "overlap_space"); err != nil || rec.Body != "Ports on the stand are y x for the probe." {
		t.Errorf("--all replaces left to right without overlap: %q %v", rec.Body, err)
	}
	if _, err := s.Edit(ctx, EditRequest{Key: "overlap_run", Old: "aaa", New: "b"}); err != nil {
		t.Errorf("a single occurrence is unique: %v", err)
	}
}

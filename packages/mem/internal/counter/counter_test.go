package counter

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestBumpAndCounts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "counters")
	got, err := Counts(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty dir: %v %v", got, err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := Bump(dir, "timeout"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := Bump(dir, "ok"); err != nil {
		t.Fatal(err)
	}
	got, err = Counts(dir)
	if err != nil || got["timeout"] != 20 || got["ok"] != 1 || len(got) != 2 {
		t.Errorf("%v %v", got, err)
	}
}

func TestBumpRejectsUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"", "../x", "a/b", ".hidden"} {
		if err := Bump(dir, name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}

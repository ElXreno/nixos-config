package session

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSeen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "0b6c4d2e-1111-4a5b-9c3d-2e2f3a4b5c6d")
	if err != nil {
		t.Fatal(err)
	}
	if s.Has("a") {
		t.Fatal("fresh session has a key")
	}
	if err := s.Add("a", "b", "a", "", "bad\nkey"); err != nil {
		t.Fatal(err)
	}
	re, err := Open(dir, "0b6c4d2e-1111-4a5b-9c3d-2e2f3a4b5c6d")
	if err != nil {
		t.Fatal(err)
	}
	got := re.Keys()
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("reopened keys = %v", got)
	}
	if f := re.Filter([]string{"a", "c", "b", "d"}); !slices.Equal(f, []string{"c", "d"}) {
		t.Fatalf("filter = %v", f)
	}
	data, _ := os.ReadFile(re.Path())
	if string(data) != "a\nb\n" {
		t.Fatalf("file = %q", data)
	}
	if err := re.Reset(); err != nil || re.Has("a") {
		t.Fatalf("reset: %v", err)
	}
	if err := re.Reset(); err != nil {
		t.Fatalf("second reset: %v", err)
	}
}

func TestFileName(t *testing.T) {
	cases := []struct {
		id   string
		safe bool
	}{
		{"0b6c4d2e-1111-4a5b-9c3d-2e2f3a4b5c6d", true},
		{"../../etc/passwd", false},
		{"a/b", false},
		{strings.Repeat("x", 200), false},
	}
	for _, c := range cases {
		name := FileName(c.id)
		if strings.ContainsRune(name, '/') || !strings.HasSuffix(name, ".keys") {
			t.Errorf("FileName(%q) = %q", c.id, name)
		}
		if (name == c.id+".keys") != c.safe {
			t.Errorf("FileName(%q) = %q, safe %v", c.id, name, c.safe)
		}
	}
	if _, err := Open(t.TempDir(), ""); err == nil {
		t.Error("empty session id accepted")
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for _, id := range []string{"old", "new"} {
		s, _ := Open(dir, id)
		if err := s.Add("k"); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(dir, SubDir, "old.keys")
	if err := os.Chtimes(old, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err := Prune(dir, 7*24*time.Hour, now)
	if err != nil || n != 1 {
		t.Fatalf("prune = %d, %v", n, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old session kept")
	}
	if _, err := os.Stat(filepath.Join(dir, SubDir, "new.keys")); err != nil {
		t.Fatal("new session removed")
	}
	if n, err := Prune(t.TempDir(), time.Hour, now); n != 0 || err != nil {
		t.Fatalf("prune of missing dir = %d, %v", n, err)
	}
}

func TestClaimIsTakenOnce(t *testing.T) {
	dir := t.TempDir()
	if Claimed(dir, "s1:agent", RecallClaim) {
		t.Fatal("claimed before any claim")
	}
	won := 0
	results := make(chan bool, 8)
	for range 8 {
		go func() {
			ok, err := Claim(dir, "s1:agent", RecallClaim)
			if err != nil {
				t.Error(err)
			}
			results <- ok
		}()
	}
	for range 8 {
		if <-results {
			won++
		}
	}
	if won != 1 || !Claimed(dir, "s1:agent", RecallClaim) {
		t.Fatalf("%d of 8 concurrent claims won", won)
	}
	if Claimed(dir, "s1:other", RecallClaim) || Claimed(dir, "s1:agent", "other") {
		t.Error("a claim leaks to another id or tag")
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(claimPath(dir, "s1:agent", RecallClaim), old, old); err != nil {
		t.Fatal(err)
	}
	if n, err := Prune(dir, 7*24*time.Hour, time.Now()); err != nil || n != 1 || Claimed(dir, "s1:agent", RecallClaim) {
		t.Fatalf("prune removed %d claim files: %v", n, err)
	}
}

func TestUnclaimReleasesOnlyItsOwnClaim(t *testing.T) {
	dir := t.TempDir()
	if err := Unclaim(dir, "s1:agent", RecallClaim); err != nil {
		t.Fatalf("unclaim without a claim: %v", err)
	}
	for _, id := range []string{"s1:agent", "s1:other"} {
		if ok, err := Claim(dir, id, RecallClaim); !ok || err != nil {
			t.Fatalf("claim %s = %v, %v", id, ok, err)
		}
	}
	if err := Unclaim(dir, "s1:agent", RecallClaim); err != nil {
		t.Fatal(err)
	}
	if Claimed(dir, "s1:agent", RecallClaim) || !Claimed(dir, "s1:other", RecallClaim) {
		t.Fatal("unclaim released the wrong claim")
	}
	if ok, err := Claim(dir, "s1:agent", RecallClaim); !ok || err != nil {
		t.Fatalf("claim after unclaim = %v, %v", ok, err)
	}
}

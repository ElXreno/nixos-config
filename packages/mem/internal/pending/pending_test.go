package pending

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func entry(agent, text string) Entry {
	return Entry{Agent: agent, Text: text, Keys: []string{"k-" + text}}
}

func TestOldestOfTheTypeFirst(t *testing.T) {
	dir := t.TempDir()
	for i, text := range []string{"first", "second", "third"} {
		if err := Enqueue(dir, "s1", entry("Explore", text), t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := Enqueue(dir, "s1", entry("Plan", "plan"), t0); err != nil {
		t.Fatal(err)
	}
	now := t0.Add(5 * time.Second)
	for _, want := range []string{"first", "second", "third"} {
		got, ok, err := Dequeue(dir, "s1", "Explore", 2*time.Minute, now)
		if err != nil || !ok || got.Text != want {
			t.Fatalf("dequeue = %q ok=%v err=%v, want %q", got.Text, ok, err, want)
		}
		if !slices.Equal(got.Keys, []string{"k-" + want}) {
			t.Errorf("keys %v", got.Keys)
		}
	}
	if _, ok, _ := Dequeue(dir, "s1", "Explore", 2*time.Minute, now); ok {
		t.Error("drained queue still answers")
	}
	if got, ok, _ := Dequeue(dir, "s1", "Plan", 2*time.Minute, now); !ok || got.Text != "plan" {
		t.Errorf("other type lost: %q %v", got.Text, ok)
	}
}

func TestSessionsAreSeparate(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("Explore", "mine"), t0); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Dequeue(dir, "s2", "Explore", time.Minute, t0); ok {
		t.Error("another session took the entry")
	}
	if _, ok, _ := Dequeue(dir, "s1", "Explore", time.Minute, t0); !ok {
		t.Error("owner lost the entry")
	}
}

func TestExpiry(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("Explore", "old"), t0); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(dir, "s1", entry("Explore", "new"), t0.Add(100*time.Second)); err != nil {
		t.Fatal(err)
	}
	now := t0.Add(2*time.Minute + time.Second)
	got, ok, err := Dequeue(dir, "s1", "Explore", 2*time.Minute, now)
	if err != nil || !ok || got.Text != "new" {
		t.Fatalf("got %q ok=%v err=%v, want the fresh entry", got.Text, ok, err)
	}
	if left := files(t, dir, "s1"); len(left) != 0 {
		t.Errorf("expired entry not removed: %v", left)
	}
	if err := Enqueue(dir, "s1", entry("Explore", "edge"), t0); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Dequeue(dir, "s1", "Explore", 2*time.Minute, t0.Add(2*time.Minute)); !ok {
		t.Error("an entry exactly at the ttl expired")
	}
}

func TestEnqueueDropsExpiredAndCaps(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("Explore", "stale"), t0); err != nil {
		t.Fatal(err)
	}
	now := t0.Add(time.Hour)
	for i := range MaxEntries + 10 {
		if err := Enqueue(dir, "s1", entry("Explore", fmt.Sprint(i)), now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	left := files(t, dir, "s1")
	if len(left) != MaxEntries {
		t.Fatalf("%d entries kept, want %d", len(left), MaxEntries)
	}
	got, ok, _ := Dequeue(dir, "s1", "Explore", time.Minute, now.Add(time.Second))
	if !ok || got.Text != "10" {
		t.Errorf("oldest kept entry %q, want the 11th enqueued", got.Text)
	}
}

func TestEmptyAndMissing(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := Dequeue(dir, "nope", "Explore", time.Minute, t0); ok || err != nil {
		t.Errorf("missing queue: ok=%v err=%v", ok, err)
	}
	if _, ok, err := Dequeue(dir, "", "Explore", time.Minute, t0); ok || err == nil {
		t.Errorf("empty session id: ok=%v err=%v", ok, err)
	}
	if err := Enqueue(dir, "", entry("Explore", "x"), t0); err == nil {
		t.Error("enqueue without a session id succeeded")
	}
	if err := Enqueue(dir, "s1", entry("", "x"), t0); err == nil {
		t.Error("enqueue without an agent type succeeded")
	}
	if err := Enqueue(dir, "s1", entry("Explore", ""), t0); err == nil {
		t.Error("enqueue of empty text succeeded")
	}
}

func TestMalformedEntriesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("Explore", "bad"), t0); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(dir, "s1", entry("Explore", "good"), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	names := files(t, dir, "s1")
	if err := os.WriteFile(filepath.Join(dir, SubDir, sessionDir("s1"), names[0]), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SubDir, sessionDir("s1"), "junk.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SubDir, sessionDir("s1"), ".tmp-1"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Dequeue(dir, "s1", "Explore", time.Minute, t0.Add(2*time.Second))
	if err != nil || !ok || got.Text != "good" {
		t.Fatalf("got %q ok=%v err=%v, want the good entry past the broken one", got.Text, ok, err)
	}
	if left := files(t, dir, "s1"); !slices.Equal(left, []string{".tmp-1", "junk.txt"}) {
		t.Errorf("malformed entry stays: %v", left)
	}
}

func TestUnsafeSessionIDsStayInside(t *testing.T) {
	dir := t.TempDir()
	id := "../../etc/x"
	if err := Enqueue(dir, id, entry("Explore", "x"), t0); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := Dequeue(dir, id, "Explore", time.Minute, t0); !ok || got.Text != "x" {
		t.Errorf("round trip failed: %q %v", got.Text, ok)
	}
	var escaped []string
	_ = filepath.WalkDir(filepath.Dir(dir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.HasPrefix(p, dir) {
			escaped = append(escaped, p)
		}
		return nil
	})
	if len(escaped) > 0 {
		t.Errorf("files outside the state dir: %v", escaped)
	}
}

func TestAgentTypeWithOddCharacters(t *testing.T) {
	dir := t.TempDir()
	odd := "plugin:Code Reviewer/ü"
	if err := Enqueue(dir, "s1", entry(odd, "x"), t0); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Dequeue(dir, "s1", "plugin:Code Reviewer", time.Minute, t0); ok {
		t.Error("a prefix of the type matched")
	}
	if got, ok, _ := Dequeue(dir, "s1", odd, time.Minute, t0); !ok || got.Text != "x" {
		t.Errorf("odd type lost: %q %v", got.Text, ok)
	}
}

func TestParallelDequeueGivesEachEntryOnce(t *testing.T) {
	dir := t.TempDir()
	const n = 12
	for i := range n {
		if err := Enqueue(dir, "s1", entry("Explore", fmt.Sprint(i)), t0.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	got := map[string]int{}
	var wg sync.WaitGroup
	for range n + 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, ok, err := Dequeue(dir, "s1", "Explore", time.Minute, t0.Add(time.Second))
			if err != nil || !ok {
				return
			}
			mu.Lock()
			got[e.Text]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(got) != n {
		t.Fatalf("%d distinct entries taken, want %d: %v", len(got), n, got)
	}
	for k, c := range got {
		if c != 1 {
			t.Errorf("entry %s taken %d times", k, c)
		}
	}
}

func TestParallelEnqueueKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Enqueue(dir, "s1", entry("Explore", fmt.Sprint(i)), t0)
		}()
	}
	wg.Wait()
	if n := len(files(t, dir, "s1")); n != 20 {
		t.Errorf("%d entries after 20 parallel enqueues at the same instant", n)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "old", entry("Explore", "x"), t0); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(dir, "new", entry("Explore", "y"), t0); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, SubDir, sessionDir("old"))
	past := t0.Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	n, err := Prune(dir, 7*24*time.Hour, t0)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d err %v, want 1", n, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old queue directory stays")
	}
	if len(files(t, dir, "new")) != 1 {
		t.Error("recent queue pruned")
	}
	if n, err := Prune(filepath.Join(dir, "missing"), time.Hour, t0); n != 0 || err != nil {
		t.Errorf("missing dir: %d %v", n, err)
	}
}

func files(t *testing.T, stateDir, sess string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(stateDir, SubDir, sessionDir(sess)))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

func TestAgentTypeCaseDoesNotMatter(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("explore", "x"), t0); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := Dequeue(dir, "s1", "Explore", time.Minute, t0); !ok || got.Text != "x" {
		t.Errorf("case-folded type lost: %q %v", got.Text, ok)
	}
}

func TestEnqueueSweepsStrayClaimAndTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("Explore", "a"), t0); err != nil {
		t.Fatal(err)
	}
	sd := filepath.Join(dir, SubDir, sessionDir("s1"))
	old := time.Now().Add(-time.Hour)
	fresh := filepath.Join(sd, ".tmp-fresh.json")
	stray := []string{filepath.Join(sd, ".tmp-stale.json"), filepath.Join(sd, "00000000000000000001-aaaaaaaaaaaa-1.json.claimed-77")}
	for _, p := range append(stray, fresh) {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range stray {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(fresh, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(dir, "s1", entry("Explore", "b"), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, p := range stray {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s survived the sweep", filepath.Base(p))
		}
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a fresh temp file of a running enqueue was removed")
	}
}

func TestAgentTypeSpellingFollowsTheHostsFolding(t *testing.T) {
	dir := t.TempDir()
	if err := Enqueue(dir, "s1", entry("general_purpose", "x"), t0); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(dir, "s1", entry("Code Reviewer", "y"), t0.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := Dequeue(dir, "s1", "General-Purpose", time.Minute, t0); !ok || got.Text != "x" {
		t.Errorf("underscore against dash: %q %v", got.Text, ok)
	}
	if got, ok, _ := Dequeue(dir, "s1", "code-reviewer", time.Minute, t0); !ok || got.Text != "y" {
		t.Errorf("space against dash: %q %v", got.Text, ok)
	}
	if got := Fold(" Fo\u2011rk_ "); got != "fork" {
		t.Errorf("fold %q", got)
	}
}

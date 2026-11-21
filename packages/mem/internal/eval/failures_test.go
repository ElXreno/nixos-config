package eval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/session"
	"mem/internal/statefile"
	"mem/internal/store"
	"mem/internal/vec"
)

func TestInjectionsAndPositives(t *testing.T) {
	dir := t.TempDir()
	turns := []statefile.Turn{
		{Time: "2026-11-08T09:00:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "inject", Strong: []statefile.Key{{Key: "strong-k"}}, Keys: []string{"strong-k"}},
		{Time: "2026-11-08T09:10:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "inject", Lexical: []string{"lex-k"}, Pointers: []statefile.Key{{Key: "ptr-k"}}},
		{Time: "2026-11-08T09:20:00Z", Event: statefile.EventTool, Session: "s1", Agent: "a1", Tool: "Bash", Status: "inject", Keys: []string{"trig-k"}},
		{Time: "2026-11-08T09:30:00Z", Event: statefile.EventTool, Session: "s1", Tool: "ToolFailure:Bash", Status: "inject", Keys: []string{"fail-ptr"}, Pointers: []statefile.Key{{Key: "fail-ptr"}}},
		{Time: "2026-11-08T11:00:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "inject", Keys: []string{"late-k"}},
		{Time: "2026-11-08T09:00:00Z", Event: statefile.EventPrompt, Session: "s2", Status: "inject", Keys: []string{"other-k"}},
		{Time: "2026-11-08T09:00:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "none"},
	}
	for _, tr := range turns {
		if err := statefile.Append(dir, statefile.TurnsName, statefile.MaxTurnsSize, tr); err != nil {
			t.Fatal(err)
		}
	}
	idx, err := loadInjections(filepath.Join(dir, statefile.TurnsName))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	plain, pointers := injectedBefore(idx["s1"], at)
	if !slices.Equal(plain, []string{"strong-k", "lex-k", "trig-k"}) || !slices.Equal(pointers, []string{"ptr-k"}) {
		t.Fatalf("plain %q pointers %q", plain, pointers)
	}
	db := map[string]store.Record{}
	for _, k := range []string{"strong-k", "late-k", "other-k", "fresh-k", "ptr-k", "fail-ptr"} {
		db[k] = store.Record{Key: k, UpdatedAt: at.Add(-time.Hour)}
	}
	db["edited-later"] = store.Record{Key: "edited-later", UpdatedAt: at.Add(time.Second)}
	db["edited-same-moment"] = store.Record{Key: "edited-same-moment", UpdatedAt: at}
	get := func(k string) (store.Record, bool, error) {
		r, ok := db[k]
		return r, ok, nil
	}
	cases := []struct {
		name                  string
		lookups               []string
		pos, already, ineligi []string
	}{
		{"strong, lexical, trigger and pointer keys were injected", []string{"strong-k", "lex-k", "trig-k", "ptr-k"}, nil, []string{"strong-k", "lex-k", "trig-k", "ptr-k"}, nil},
		{"a live failure hook's pointer is recomputed by the replay", []string{"fail-ptr"}, []string{"fail-ptr"}, nil, nil},
		{"injected after the failure or in another session", []string{"late-k", "other-k"}, []string{"late-k", "other-k"}, nil, nil},
		{"missing or edited after the failure", []string{"gone", "edited-later", "edited-same-moment", "fresh-k"}, []string{"edited-same-moment", "fresh-k"}, nil, []string{"gone", "edited-later"}},
		{"nothing looked up", nil, nil, nil, nil},
	}
	for _, c := range cases {
		pos, already, inel, err := splitPositives(c.lookups, plain, pointers, at, get)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(pos, c.pos) || !slices.Equal(already, c.already) || !slices.Equal(inel, c.ineligi) {
			t.Errorf("%s: positives %q already %q ineligible %q", c.name, pos, already, inel)
		}
	}
	if _, err := loadInjections(filepath.Join(dir, "missing.jsonl")); err == nil {
		t.Error("a missing --turns file must fail")
	}
}

func appendTurns(t *testing.T, dir string, turns ...statefile.Turn) string {
	t.Helper()
	for _, tr := range turns {
		if err := statefile.Append(dir, statefile.TurnsName, statefile.MaxTurnsSize, tr); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, statefile.TurnsName)
}

func TestLiveFailureTurnsAreNotInjectedBefore(t *testing.T) {
	path := appendTurns(t, t.TempDir(),
		statefile.Turn{Time: "2026-11-08T09:10:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "inject", Pointers: []statefile.Key{{Key: "ptr-k"}}},
		statefile.Turn{Time: "2026-11-08T09:59:59Z", Event: statefile.EventTool, Session: "s1", Tool: "ToolFailure:Bash", Status: "inject", Keys: []string{"prev-second"}, Pointers: []statefile.Key{{Key: "prev-second"}}},
		statefile.Turn{Time: "2026-11-08T10:00:00Z", Event: statefile.EventTool, Session: "s1", Tool: "ToolFailure:Bash", Status: "inject", Keys: []string{"live-ptr"}, Pointers: []statefile.Key{{Key: "live-ptr"}}},
	)
	idx, err := loadInjections(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 8, 10, 0, 0, 400_000_000, time.UTC)
	plain, pointers := injectedBefore(idx["s1"], at)
	get := func(k string) (store.Record, bool, error) {
		return store.Record{Key: k, UpdatedAt: at.Add(-time.Hour)}, true, nil
	}
	pos, already, _, err := splitPositives([]string{"ptr-k", "live-ptr", "prev-second"}, plain, pointers, at, get)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != 0 || !slices.Equal(pointers, []string{"ptr-k"}) || !slices.Equal(pos, []string{"live-ptr", "prev-second"}) || !slices.Equal(already, []string{"ptr-k"}) {
		t.Fatalf("plain %q pointers %q positives %q already %q", plain, pointers, pos, already)
	}
}

func TestSubagentStartPointerKeysAreAlreadyInjected(t *testing.T) {
	path := appendTurns(t, t.TempDir(),
		statefile.Turn{Time: "2026-11-08T09:00:00Z", Event: statefile.EventTool, Session: "s1", Agent: "a1", Tool: "SubagentStart:general-purpose", Status: "inject", Keys: []string{"full-k", "~ptr:ptr-k", "~map", "~trig:x"}},
	)
	idx, err := loadInjections(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	plain, pointers := injectedBefore(idx["s1"], at)
	get := func(k string) (store.Record, bool, error) {
		return store.Record{Key: k, UpdatedAt: at.Add(-time.Hour)}, true, nil
	}
	pos, already, _, err := splitPositives([]string{"full-k", "ptr-k"}, plain, pointers, at, get)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plain, []string{"full-k"}) || !slices.Equal(pointers, []string{"ptr-k"}) || len(pos) != 0 || !slices.Equal(already, []string{"full-k", "ptr-k"}) {
		t.Fatalf("plain %q pointers %q positives %q already %q", plain, pointers, pos, already)
	}
}

func TestSnapshotDBUsesTheSourceWhenItIsTheTarget(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "out", FailureDBDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(filepath.Join(dir, env.DBName)+s, []byte("data"+s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, src := range []string{filepath.Join(dir, env.DBName), filepath.Join(link, env.DBName)} {
		got, err := snapshotDB(src, dir)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		gi, gerr := os.Stat(got)
		si, serr := os.Stat(src)
		if gerr != nil || serr != nil || !os.SameFile(gi, si) {
			t.Errorf("%s: snapshot %s is not the source: %v %v", src, got, gerr, serr)
		}
		for _, s := range []string{"", "-wal", "-shm"} {
			b, err := os.ReadFile(filepath.Join(dir, env.DBName) + s)
			if err != nil || string(b) != "data"+s {
				t.Errorf("%s: source%s lost or changed: %q %v", src, s, b, err)
			}
		}
	}
}

func TestSnapshotDBReplacesAStaleCopy(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", env.DBName)
	dir := filepath.Join(root, "out", FailureDBDir)
	for _, d := range []string{filepath.Dir(src), dir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{src: "fresh", filepath.Join(dir, env.DBName): "stale", filepath.Join(dir, env.DBName) + "-wal": "stale-wal"}
	for p, v := range files {
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := snapshotDB(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(got); err != nil || string(b) != "fresh" || got != filepath.Join(dir, env.DBName) {
		t.Errorf("snapshot %s: %q %v", got, b, err)
	}
	if _, err := os.Stat(got + "-wal"); err == nil {
		t.Error("a stale -wal survived next to the copy")
	}
}

func TestSampleEvenly(t *testing.T) {
	xs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	cases := []struct {
		max     int
		want    []int
		dropped int
	}{
		{4, []int{0, 2, 5, 7}, 6},
		{5, []int{0, 2, 4, 6, 8}, 5},
		{1, []int{0}, 9},
		{10, xs, 0},
		{500, xs, 0},
	}
	for _, c := range cases {
		got, dropped := sampleEvenly(xs, c.max)
		if !slices.Equal(got, c.want) || dropped != c.dropped {
			t.Errorf("max %d: %v dropped %d", c.max, got, dropped)
		}
		again, _ := sampleEvenly(xs, c.max)
		if !slices.Equal(got, again) {
			t.Errorf("max %d: not deterministic", c.max)
		}
	}
}

func TestFailureVerdict(t *testing.T) {
	cases := []struct {
		name               string
		r                  FailuresReport
		verdict, reasonHas string
	}{
		{"dry run", FailuresReport{DryRun: true, PositiveEvents: 100, HitEvents: 90, InjectedEvents: 90}, VerdictNotRun, "dry run"},
		{"too few positives", FailuresReport{PositiveEvents: 19, HitEvents: 19, InjectedEvents: 19}, VerdictInconclusive, "only 19 failures were followed by a lookup of an eligible record"},
		{"none at all", FailuresReport{}, VerdictInconclusive, "only 0 failures"},
		{"both thresholds met exactly", FailuresReport{PositiveEvents: 20, HitEvents: 6, InjectedEvents: 30}, VerdictHelps, "6 of 20"},
		{"too much noise", FailuresReport{PositiveEvents: 20, HitEvents: 6, InjectedEvents: 31}, VerdictNoHelp, "6 of 31"},
		{"too few hits", FailuresReport{PositiveEvents: 20, HitEvents: 5, InjectedEvents: 10}, VerdictNoHelp, "5 of 20"},
		{"no injections", FailuresReport{PositiveEvents: 40}, VerdictNoHelp, "0 of 0"},
		{"clear help", FailuresReport{PositiveEvents: 50, HitEvents: 30, InjectedEvents: 60}, VerdictHelps, "30 of 50"},
	}
	for _, c := range cases {
		v, why := failureVerdict(c.r)
		if v != c.verdict || !strings.Contains(why, c.reasonHas) {
			t.Errorf("%s: %q %q", c.name, v, why)
		}
	}
}

func TestPointerKeys(t *testing.T) {
	out := `{"hookSpecificOutput":{"hookEventName":"PostToolUseFailure","additionalContext":"Agent memory: possibly related to this failure, not loaded (` + "`mem get KEY`" + ` prints one).\n- ` + "`zfs-arc-ram`" + ` [fact, as of 2026-10-01]: ZFS ARC\n- ` + "`bt#2`" + ` [log, as of 2026-10-02]: x"}}`
	if got := pointerKeys([]byte(out)); !slices.Equal(got, []string{"zfs-arc-ram", "bt#2"}) {
		t.Errorf("%q", got)
	}
	for _, empty := range []string{"", "\n", "not json"} {
		if got := pointerKeys([]byte(empty)); got != nil {
			t.Errorf("%q: %q", empty, got)
		}
	}
}

func replayVec(text string) []float64 {
	lower := strings.ToLower(text)
	v := []float64{0, 0, 0.1}
	for axis, words := range [][]string{{"arc", "zfs"}, {"bluetooth"}} {
		for _, w := range words {
			if strings.Contains(lower, w) {
				v[axis] = 1
			}
		}
	}
	n := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	for i := range v {
		v[i] /= n
	}
	return v
}

func replayEmbedder(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad", 400)
			return
		}
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		var data []item
		for i, in := range req.Input {
			data = append(data, item{i, replayVec(in)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dirDigest(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		out[e.Name()] = hex.EncodeToString(sum[:])
	}
	return out
}

func readEvents(t *testing.T, path string) []FailureEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []FailureEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var ev FailureEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("%v: %s", err, line)
		}
		out = append(out, ev)
	}
	return out
}

func TestRunFailuresEndToEnd(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	seeded := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(root, "db", "mem.db")
	s, err := store.Open(ctx, dbPath, store.Options{Now: func() time.Time { return seeded }})
	if err != nil {
		t.Fatal(err)
	}
	recs := []store.Record{
		{Key: "zfs-arc-ram", Kind: store.Fact, Scopes: []string{"global"}, Title: "ZFS ARC size after boot", Body: "The ZFS ARC grows to half of RAM when zfs_arc_max is unset.", UpdatedAt: seeded},
		{Key: "bt-pairing", Kind: store.Fact, Scopes: []string{"global"}, Title: "Bluetooth pairing", Body: "The bluetooth headset pairs only after a controller reset.", UpdatedAt: seeded},
	}
	if _, err := s.Upsert(ctx, recs, nil, "test"); err != nil {
		t.Fatal(err)
	}
	srv := replayEmbedder(t)
	if _, err := recall.Reindex(ctx, s, recall.ReindexOptions{Model: "m", DocPrefix: "Document: ", Embedder: vec.NewClient(vec.Options{URL: srv.URL, Model: "m"}), Batch: 4, Par: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	before := dirDigest(t, filepath.Dir(dbPath))

	projects := filepath.Join(root, "home", ".claude", "projects", "p")
	work := filepath.Join(root, "work")
	for _, d := range []string{filepath.Join(projects, "s1", "subagents"), work} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	main := jsonlOf(t,
		bashUse("t1", "zpool import tank", "2026-11-08T10:00:00.100Z"),
		bashResult("t1", "Exit code 1\ncannot import 'tank': out of memory, the zfs arc did not shrink in time", true, "2026-11-08T10:00:00.500Z"),
		bashUse("t2", "ls", "2026-11-08T10:00:10Z"),
		bashUse("t3", "mem get zfs-arc-ram | head", "2026-11-08T10:00:20Z"),
		bashUse("t4", "mem get --body bt-pairing missing-key", "2026-11-08T10:00:30Z"),
	)
	sub := jsonlOf(t,
		withAgent(bashUse("t1", "make all", "2026-11-08T10:05:00Z"), "a1"),
		withAgent(bashResult("t1", "Exit code 2\nmake: *** No rule to make target 'all'.  Stop.", true, "2026-11-08T10:05:01Z"), "a1"),
		withAgent(bashUse("t2", "ls", "2026-11-08T10:05:10Z"), "a1"),
	)
	main = strings.ReplaceAll(main, `"cwd":"/w"`, `"cwd":"`+work+`"`)
	sub = strings.ReplaceAll(sub, `"cwd":"/w"`, `"cwd":"`+work+`"`)
	if err := os.WriteFile(filepath.Join(projects, "s1.jsonl"), []byte(main), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projects, "s1", "subagents", "agent-a1.jsonl"), []byte(sub), 0o600); err != nil {
		t.Fatal(err)
	}
	turnsDir := filepath.Join(root, "turns")
	if err := statefile.Append(turnsDir, statefile.TurnsName, statefile.MaxTurnsSize, statefile.Turn{Time: "2026-11-08T09:59:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "inject", Strong: []statefile.Key{{Key: "bt-pairing"}}, Keys: []string{"bt-pairing"}}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	vars := map[string]string{"HOME": filepath.Join(root, "home"), "XDG_STATE_HOME": filepath.Join(root, "real-state"), "MEM_DB": filepath.Join(root, "elsewhere.db")}
	var stdout, stderr bytes.Buffer
	e := env.Env{Stdout: &stdout, Stderr: &stderr, Getenv: func(k string) string { return vars[k] }, Now: time.Now,
		Getwd: func() (string, error) { return work, nil }, Hostname: func() (string, error) { return "testhost", nil }}
	args := []string{"failures", "--db", dbPath, "--transcripts", "~/.claude/projects/**/*.jsonl", "--turns", filepath.Join(turnsDir, statefile.TurnsName),
		"--out", out, "--since", "2026-11-07", "--", "--embed-url", srv.URL, "--embed-model", "m", "--vec-pool", "reg", "--reg-cos", "0.6"}
	if code := Run(args, e); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	evs := readEvents(t, filepath.Join(out, FailureEventsName))
	if len(evs) != 2 {
		t.Fatalf("%d events", len(evs))
	}
	first, second := evs[0], evs[1]
	if first.Session != "s1" || first.Agent != "" || first.Time != "2026-11-08T10:00:00.5Z" || first.Command != "zpool import tank" ||
		!strings.HasSuffix(first.Error, "did not shrink in time") || !slices.Equal(first.Injected, []string{"zfs-arc-ram"}) ||
		!slices.Equal(first.Positives, []string{"zfs-arc-ram"}) || !slices.Equal(first.AlreadyInjected, []string{"bt-pairing"}) ||
		!slices.Equal(first.Ineligible, []string{"missing-key"}) || !slices.Equal(first.Hits, []string{"zfs-arc-ram"}) {
		t.Errorf("first event %+v", first)
	}
	if second.Agent != "a1" || second.Command != "make all" || len(second.Injected) != 0 || len(second.Positives) != 0 || len(second.Hits) != 0 || second.Hits == nil {
		t.Errorf("second event %+v", second)
	}
	data, err := os.ReadFile(filepath.Join(out, FailureReportName))
	if err != nil {
		t.Fatal(err)
	}
	var rep FailuresReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	want := FailuresReport{Since: "2026-11-07", DB: dbPath, Matched: 2, Files: 2, Failures: 2, Replayed: 2, InjectedEvents: 1, InjectedPointers: 1, PositiveEvents: 1, Positives: 1,
		HitEvents: 1, HitKeys: 1, AlreadyInjected: 1, Ineligible: 1, Verdict: VerdictInconclusive, Reason: "only 1 failures were followed by a lookup of an eligible record"}
	got := rep
	got.Latency = FailureLatency{}
	if got != want {
		t.Errorf("report\n%+v\nwant\n%+v", got, want)
	}
	if rep.Latency.Max < rep.Latency.P90 || rep.Latency.P90 < rep.Latency.P50 {
		t.Errorf("latency %+v", rep.Latency)
	}
	for _, s := range []string{"failures since 2026-11-07: 2, replayed 2", "hits 1 events, 1 keys", "verdict: inconclusive (only 1 failures"} {
		if !strings.Contains(stdout.String(), s) {
			t.Errorf("stdout lacks %q:\n%s", s, stdout.String())
		}
	}
	stateDir := filepath.Join(out, FailureStateDir, env.DirName)
	main1, err := session.Open(stateDir, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !main1.Has("bt-pairing") || !main1.Has("~ptr:zfs-arc-ram") || !slices.ContainsFunc(main1.Keys(), func(k string) bool { return strings.HasPrefix(k, "~fail:") }) {
		t.Errorf("main session state %q", main1.Keys())
	}
	if _, err := os.Stat(filepath.Join(stateDir, session.SubDir, session.FileName("s1:a1"))); err != nil {
		t.Errorf("subagent session state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "hook.log")); err != nil {
		t.Errorf("hook.log in the private state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, statefile.TurnsName)); err == nil {
		t.Error("the replay wrote turns.jsonl")
	}
	for _, p := range []string{filepath.Join(root, "real-state"), filepath.Join(root, "elsewhere.db")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("the replay touched %s", p)
		}
	}
	if after := dirDigest(t, filepath.Dir(dbPath)); len(after) != len(before) || !mapsEqual(after, before) {
		t.Errorf("database dir changed:\nbefore %v\nafter  %v", before, after)
	}
	if snap := dirDigest(t, filepath.Join(out, FailureDBDir)); snap[env.DBName] != before[env.DBName] {
		t.Errorf("snapshot %v, source %v", snap, before)
	}

	dry := filepath.Join(root, "dry")
	stdout.Reset()
	if code := Run([]string{"failures", "--db", dbPath, "--transcripts", "~/.claude/projects/**/*.jsonl", "--turns", filepath.Join(turnsDir, statefile.TurnsName),
		"--out", dry, "--dry-run", "--max", "1"}, e); code != 0 {
		t.Fatalf("dry run exit %d: %s", code, stderr.String())
	}
	data, err = os.ReadFile(filepath.Join(dry, FailureReportName))
	if err != nil {
		t.Fatal(err)
	}
	var drep FailuresReport
	if err := json.Unmarshal(data, &drep); err != nil {
		t.Fatal(err)
	}
	if !drep.DryRun || drep.Verdict != VerdictNotRun || drep.Failures != 2 || drep.Replayed != 1 || drep.DroppedByMax != 1 || drep.PositiveEvents != 1 || drep.InjectedEvents != 0 {
		t.Errorf("dry run report %+v", drep)
	}
	if _, err := os.Stat(filepath.Join(dry, FailureStateDir)); err == nil {
		t.Error("the dry run made a state dir")
	}
	if !strings.Contains(stdout.String(), "hook: not called (--dry-run)") {
		t.Errorf("dry run stdout:\n%s", stdout.String())
	}
}

func mapsEqual(a, b map[string]string) bool {
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func failuresHome(t *testing.T) (env.Env, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	vars := map[string]string{"HOME": home}
	e := env.Env{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Getenv: func(k string) string { return vars[k] }, Now: time.Now,
		Getwd: func() (string, error) { return root, nil }, Hostname: func() (string, error) { return "testhost", nil }}
	s, err := store.Open(context.Background(), e.DBPath(), store.Options{Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(home, ".claude", "projects", "p")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	tr := jsonlOf(t, bashUse("t1", "make", "2026-11-08T10:00:00Z"), bashResult("t1", "Exit code 2\nmake: no rule", true, "2026-11-08T10:00:01Z"))
	if err := os.WriteFile(filepath.Join(projects, "s1.jsonl"), []byte(tr), 0o600); err != nil {
		t.Fatal(err)
	}
	turns := appendTurns(t, e.StateDir(), statefile.Turn{Time: "2026-11-08T09:59:00Z", Event: statefile.EventPrompt, Session: "s1", Status: "inject", Keys: []string{"k"}})
	return e, turns
}

func TestRunFailuresKeepsRealStateTree(t *testing.T) {
	e, turns := failuresHome(t)
	other := filepath.Join(e.Home(), ".local", "state", "otherapp", "data")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"failures", "--transcripts", "~/.claude/projects/**/*.jsonl", "--turns", turns, "--out", "~/.local"}
	for run := range 2 {
		if code := Run(args, e); code != 0 {
			t.Fatalf("run %d: exit %d: %s", run, code, e.Stderr.(*bytes.Buffer).String())
		}
	}
	for _, p := range []string{turns, other, e.DBPath()} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("mem eval failures --out ~/.local destroyed %s: %v", p, err)
		}
	}
}

func TestRunFailuresRefusesAStateDirItDidNotCreate(t *testing.T) {
	e, turns := failuresHome(t)
	out := filepath.Join(e.Home(), "out")
	foreign := filepath.Join(out, FailureStateDir, "keep")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"failures", "--transcripts", "~/.claude/projects/**/*.jsonl", "--turns", turns, "--out", out}, e)
	if stderr := e.Stderr.(*bytes.Buffer).String(); code == 0 || !strings.Contains(stderr, "refusing to delete it") {
		t.Errorf("exit %d: %s", code, stderr)
	}
	if b, err := os.ReadFile(foreign); err != nil || string(b) != "x" {
		t.Errorf("foreign state dir changed: %q %v", b, err)
	}
}

func TestRunFailuresFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		err  string
	}{
		{"missing out", []string{"failures", "--transcripts", "x", "--turns", "y"}, "are required"},
		{"bad since", []string{"failures", "--transcripts", "x", "--turns", "y", "--out", "z", "--since", "yesterday"}, "--since"},
		{"bad max", []string{"failures", "--transcripts", "x", "--turns", "y", "--out", "z", "--max", "0"}, "at least 1"},
	}
	for _, c := range cases {
		var stderr bytes.Buffer
		e := env.Env{Stdout: &bytes.Buffer{}, Stderr: &stderr, Getenv: func(string) string { return "" }, Now: time.Now}
		if code := Run(c.args, e); code != 2 || !strings.Contains(stderr.String(), c.err) {
			t.Errorf("%s: %d %q", c.name, code, stderr.String())
		}
	}
}

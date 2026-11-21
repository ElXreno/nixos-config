package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/recall"
)

func goldAt(m int) string {
	return time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC).Add(time.Duration(m) * time.Minute).Format("2006-01-02T15:04:05.000Z")
}

func memFile(name, text string) string {
	return "---\nname: " + name + "\ndescription: " + text + "\ntype: feedback\n---\n" + text + "\n"
}

type goldFixture struct {
	root, home, mem, projects string
	e                         env.Env
	stdout, stderr            *bytes.Buffer
}

func newGoldFixture(t *testing.T) *goldFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	projects := filepath.Join(home, ".claude", "projects", "-home-u-repo")
	mem := filepath.Join(projects, "memory")
	files := map[string]string{"MEMORY.md": "- feedback_x · feedback_y\n"}
	for _, n := range []string{"x", "y", "z", "w", "b", "s", "n"} {
		files["feedback_"+n+".md"] = memFile("feedback_"+n, "Rule "+n+" about the flake inputs and the kernel config")
	}
	writeFiles(t, mem, files)
	mtime := func(slug string, m int) {
		at := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC).Add(time.Duration(m) * time.Minute)
		if err := os.Chtimes(filepath.Join(mem, slug+".md"), at, at); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"x", "w", "b", "s", "n"} {
		mtime("feedback_"+s, -48*60)
	}
	mtime("feedback_y", 13)
	mtime("feedback_z", 180)
	read := func(slug string, m int) map[string]any {
		return stamped(toolUse("Read", filepath.Join(mem, "feedback_"+slug+".md"), false), goldAt(m))
	}
	bash := func(cmd string, m int) map[string]any {
		return toolCall("Bash", map[string]any{"command": cmd}, goldAt(m))
	}
	m := "~/.claude/projects/-home-u-repo/memory"
	x := &tx{t: t}
	x.add(stamped(human("turn one about the flake inputs and rule x"), goldAt(0)))
	x.add(read("x", 1))
	x.add(stamped(human("turn two about the kernel config and rule y"), goldAt(10)))
	x.add(read("y", 11))
	x.add(toolCall("Edit", map[string]any{"file_path": filepath.Join(mem, "feedback_y.md"), "old_string": "a", "new_string": "b"}, goldAt(12)))
	x.add(stamped(human("turn three about the build cache and rule z"), goldAt(20)))
	x.add(read("z", 21))
	x.add(stamped(human("turn four about the sync folder and rule w"), goldAt(30)))
	x.add(read("w", 31))
	x.add(stamped(human("turn five that edits rule w"), goldAt(40)))
	x.add(toolCall("Edit", map[string]any{"file_path": filepath.Join(mem, "feedback_w.md"), "old_string": "a", "new_string": "b"}, goldAt(41)))
	x.add(stamped(human("turn six that reads rule b with the shell"), goldAt(50)))
	x.add(bash("cat "+m+"/feedback_b.md | head -20", 51))
	x.add(stamped(human("turn seven that delegates the lookup of rule s"), goldAt(60)))
	x.add(toolCall("Agent", map[string]any{"prompt": "look up rule s"}, goldAt(61)))
	x.add(stamped(human("turn eight that reads and rewrites rule b"), goldAt(70)))
	x.add(bash("cat "+m+"/feedback_b.md", 71))
	x.add(bash("sed -i 's/a/b/' "+m+"/feedback_b.md", 72))
	x.add(stamped(human("turn nine without any memory access at all"), goldAt(80)))
	x.add(stamped(assistantText("Nothing to look up."), goldAt(81)))
	x.add(stamped(human("turn ten that only appends to rule n"), goldAt(90)))
	x.add(bash("echo more >> "+m+"/feedback_n.md", 91))
	x.add(stamped(human("turn eleven that reads the index with the shell"), goldAt(100)))
	x.add(bash("cat "+m+"/MEMORY.md", 101))
	sub := &tx{t: t}
	sub.add(map[string]any{"type": "user", "isSidechain": true, "timestamp": goldAt(61), "message": map[string]any{"role": "user", "content": "find it"}})
	sub.add(stamped(toolUse("Read", filepath.Join(mem, "feedback_s.md"), true), goldAt(62)))
	sub.add(stamped(toolUse("Read", filepath.Join(mem, "MEMORY.md"), true), goldAt(63)))
	writeFiles(t, projects, map[string]string{
		"n.jsonl":                   x.String(),
		"n/subagents/agent-1.jsonl": sub.String(),
	})
	vars := map[string]string{"HOME": home}
	f := &goldFixture{root: root, home: home, mem: mem, projects: projects, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	f.e = env.Env{Stdin: strings.NewReader(""), Stdout: f.stdout, Stderr: f.stderr, Getenv: func(k string) string { return vars[k] },
		Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }, Hostname: func() (string, error) { return "smok", nil }}
	return f
}

func (f *goldFixture) config(g GoldOptions) Config {
	return Config{Glob: filepath.Join(f.projects, "*.jsonl"), LegacyDir: f.mem, Project: "repo", Out: filepath.Join(f.root, "out"),
		Limit: DefaultLimit, Depth: DefaultDepth, Tail: DefaultTail, MinChars: DefaultMinChars, MaxQuery: DefaultMaxQuery, Seed: DefaultSeed,
		Recall: recall.DefaultSettings(), Gold: g}
}

func goldOf(t *testing.T, f *goldFixture, g GoldOptions) []string {
	t.Helper()
	p, err := prepare(t.Context(), f.e, f.config(g))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.s.Close() }()
	var out []string
	for _, q := range p.qs {
		for _, s := range q.gold {
			out = append(out, fmt.Sprintf("%d:%s", q.Turn, strings.TrimPrefix(s, "feedback_")))
		}
	}
	slices.Sort(out)
	return out
}

func gopt(tier int, dates string, d, b, s bool) GoldOptions {
	return GoldOptions{Tier: tier, Dates: dates, Direct: d, Bash: b, Sub: s}
}

func TestGoldTiersAndSources(t *testing.T) {
	f := newGoldFixture(t)
	cases := []struct {
		name string
		opt  GoldOptions
		want []string
	}{
		{"tier0 direct", gopt(0, DatesDB, true, false, false), []string{"1:x", "2:y", "3:z", "4:w"}},
		{"tier1 direct", gopt(1, DatesDB, true, false, false), []string{"1:x", "3:z", "4:w"}},
		{"tier2 db direct", gopt(2, DatesDB, true, false, false), []string{"1:x", "4:w"}},
		{"tier2 events direct", gopt(2, DatesEvents, true, false, false), []string{"1:x", "3:z"}},
		{"tier2 both direct", gopt(2, DatesBoth, true, false, false), []string{"1:x"}},
		{"tier0 bash only", gopt(0, DatesDB, false, true, false), []string{"6:b", "8:b"}},
		{"tier1 bash only", gopt(1, DatesDB, false, true, false), []string{"6:b"}},
		{"tier2 db bash only", gopt(2, DatesDB, false, true, false), []string{"6:b"}},
		{"tier2 events bash only", gopt(2, DatesEvents, false, true, false), nil},
		{"tier0 subagent only", gopt(0, DatesDB, false, false, true), []string{"7:s"}},
		{"tier2 both subagent only", gopt(2, DatesBoth, false, false, true), []string{"7:s"}},
		{"tier0 all", gopt(0, DatesDB, true, true, true), []string{"1:x", "2:y", "3:z", "4:w", "6:b", "7:s", "8:b"}},
		{"tier1 all", gopt(1, DatesDB, true, true, true), []string{"1:x", "3:z", "4:w", "6:b", "7:s"}},
		{"tier2 both all", gopt(2, DatesBoth, true, true, true), []string{"1:x", "7:s"}},
	}
	for _, c := range cases {
		if got := goldOf(t, f, c.opt); !slices.Equal(got, c.want) {
			t.Errorf("%s: gold %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGoldNegativesDoNotDependOnTier(t *testing.T) {
	f := newGoldFixture(t)
	for _, g := range []GoldOptions{gopt(0, DatesDB, true, false, false), gopt(1, DatesDB, true, true, true), gopt(2, DatesBoth, true, true, true)} {
		p, err := prepare(t.Context(), f.e, f.config(g))
		if err != nil {
			t.Fatal(err)
		}
		var turns []int
		for _, q := range p.negs {
			turns = append(turns, q.Turn)
		}
		_ = p.s.Close()
		if !slices.Equal(turns, []int{9}) || p.sum.NegativePool != 1 {
			t.Errorf("%s/%s: negative turns %v pool %d, want only the clean turn 9 (edit targets, appends and index reads are not negatives)", g.TierName(), g.Sources(), turns, p.sum.NegativePool)
		}
	}
}

func TestGoldMatrixAndWriteEvents(t *testing.T) {
	f := newGoldFixture(t)
	cfg := f.config(gopt(0, DatesDB, true, false, false))
	cfg.GoldMatrix = filepath.Join(f.root, "matrix.tsv")
	cfg.WriteEvents = filepath.Join(f.root, "writes.tsv")
	p, err := prepare(t.Context(), f.e, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.s.Close()
	rep := p.gold
	if rep == nil || len(rep.Matrix) != 7*7 {
		t.Fatalf("report %+v", rep)
	}
	row := func(tier, sources string) MatrixRow {
		for _, r := range rep.Matrix {
			if r.Tier == tier && r.Sources == sources {
				return r
			}
		}
		t.Fatalf("no row %s/%s", tier, sources)
		return MatrixRow{}
	}
	t0 := row("tier0", "direct")
	if t0.Prompts != 4 || t0.Sessions != 1 || t0.GoldPairs != 4 || t0.InTier0 != 4 || t0.NewVsTier0 != 0 || t0.Negatives != 1 {
		t.Errorf("tier0 direct %+v", t0)
	}
	if r := row("tier1", "direct"); r.Prompts != 3 || r.DroppedSameTurn != 1 || r.DroppedPrompts != 1 || r.InTier0 != 3 || r.PairsInTier0 != 3 {
		t.Errorf("tier1 direct %+v", r)
	}
	if r := row("tier2-db", "direct"); r.Prompts != 2 || r.DroppedLater != 1 {
		t.Errorf("tier2 db direct %+v", r)
	}
	if r := row("tier2-both", "direct+bash+subagent"); r.Prompts != 2 || r.NewVsTier0 != 1 || r.InTier0 != 1 || r.Sessions != 1 {
		t.Errorf("tier2 both all %+v", r)
	}
	if r := row("tier0", "direct+bash+subagent"); r.Prompts != 7 || r.NewVsTier0 != 3 || r.TouchedNotGold != 3 {
		t.Errorf("tier0 all %+v", r)
	}
	if r := row("tier0", "bash"); r.Prompts != 2 || r.InTier0 != 0 {
		t.Errorf("tier0 bash %+v", r)
	}
	w := rep.Writes
	byTool := map[string]ToolCount{}
	for _, tc := range w.Tools {
		byTool[tc.Tool] = tc
	}
	if w.Events != 4 || byTool["Edit"].Main != 2 || byTool["Bash"].Main != 2 || byTool["Bash"].Sub != 0 || w.Files != 4 {
		t.Errorf("write stats %+v", w)
	}
	ops := map[string]int{}
	for _, tc := range w.BashByOp {
		ops[tc.Tool] = tc.Main
	}
	if ops["sed-i"] != 1 || ops["redirect"] != 1 || len(ops) != 2 {
		t.Errorf("bash ops %v", ops)
	}
	c := rep.Counts
	if c.BashCalls != 5 || c.BashReads != 3 || c.BashWrites != 2 || c.BashNoEffect != 0 || c.SubReads != 1 || c.SubIndexReads != 1 {
		t.Errorf("counts %+v", c)
	}
	for _, name := range []string{"matrix.tsv", "writes.tsv"} {
		info, err := os.Stat(filepath.Join(f.root, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", name, info, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "writes.tsv"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 || lines[0] != "session\ttime\tslug\ttool\tvia\tindex" || !strings.HasPrefix(lines[1], "n\t2026-11-06T10:12:00Z\tfeedback_y\tEdit\tmain\t0") {
		t.Errorf("write events:\n%s", data)
	}
}

func TestGoldSubagentAttributionUsesTheParentTurn(t *testing.T) {
	mem := "/home/u/.claude/projects/-home-u-repo/memory"
	lg := Legacy{Dirs: []string{mem}, Indexes: map[string]bool{"MEMORY": true}}
	x := &tx{t: t}
	x.add(stamped(human("first turn that asks for a lookup of the rule"), goldAt(0)))
	x.add(stamped(human("second turn that asks for another lookup of the rule"), goldAt(10)))
	got, err := ParseTranscriptFull(strings.NewReader(x.String()), "s", lg, ParseOptions{SubAccess: []Access{
		{At: time.Date(2026, 11, 6, 10, 5, 0, 0, time.UTC), Slug: "a", Kind: AccessRead, Tool: "Read", Sub: true},
		{At: time.Date(2026, 11, 6, 10, 6, 0, 0, time.UTC), Slug: "b", Kind: AccessBashRead, Tool: "Bash:cat", Sub: true},
		{At: time.Date(2026, 11, 6, 10, 7, 0, 0, time.UTC), Slug: "c", Kind: AccessWrite, Tool: "Edit", Sub: true},
		{At: time.Date(2026, 11, 6, 10, 8, 0, 0, time.UTC), Slug: "MEMORY", Kind: AccessRead, Tool: "Read", Sub: true, Index: true},
		{At: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC), Slug: "d", Kind: AccessRead, Tool: "Read", Sub: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p := got.Prompts
	if len(p) != 2 || !slices.Equal(p[0].SubGold, []string{"a"}) || !slices.Equal(p[0].BashGold, []string{"b"}) || !slices.Equal(p[0].WrittenAll, []string{"c"}) || len(p[0].Gold) != 0 || len(p[1].SubGold) != 0 {
		t.Fatalf("prompts %+v", p)
	}
	if got.Gold.SubReads != 1 || got.Gold.SubBashReads != 1 || got.Gold.SubWrites != 1 || got.Gold.SubIndexReads != 1 || got.Gold.SubUnattributed != 1 || len(got.Writes) != 1 || !got.Writes[0].Sub {
		t.Errorf("gold counts %+v writes %+v", got.Gold, got.Writes)
	}
	if !p[0].Start.Equal(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("start %v", p[0].Start)
	}
}

func TestGoldWrittenAfterBoundaries(t *testing.T) {
	start := time.Date(2026, 11, 6, 10, 0, 30, 400_000_000, time.UTC)
	env := &goldEnv{dbDate: map[string]time.Time{
		"older":   time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC),
		"sameSec": time.Date(2026, 11, 6, 10, 0, 30, 0, time.UTC),
		"next":    time.Date(2026, 11, 6, 10, 0, 31, 0, time.UTC),
	}, events: map[string][]time.Time{
		"ev":     {time.Date(2026, 11, 6, 10, 0, 30, 400_000_000, time.UTC)},
		"before": {time.Date(2026, 11, 6, 10, 0, 30, 399_000_000, time.UTC)},
	}}
	for _, c := range []struct {
		slug, mode string
		want       bool
	}{
		{"older", DatesDB, false}, {"sameSec", DatesDB, true}, {"next", DatesDB, true}, {"missing", DatesDB, false},
		{"ev", DatesEvents, true}, {"before", DatesEvents, false}, {"older", DatesEvents, false}, {"next", DatesEvents, false},
		{"ev", DatesBoth, true}, {"next", DatesBoth, true}, {"older", DatesBoth, false},
	} {
		if got := env.writtenAfter(c.slug, start, c.mode); got != c.want {
			t.Errorf("%s/%s: %v, want %v", c.slug, c.mode, got, c.want)
		}
	}
	if env.writtenAfter("next", time.Time{}, DatesDB) {
		t.Error("a prompt without a timestamp cannot be placed")
	}
}

func TestGoldEventsAreBoundedByTheImport(t *testing.T) {
	imp := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	env := newGoldEnv(nil, []Access{
		{Slug: "a", Kind: AccessWrite, At: imp.Add(-time.Hour)},
		{Slug: "a", Kind: AccessWrite, At: imp.Add(time.Hour)},
		{Slug: "b", Kind: AccessWrite, At: imp.Add(time.Minute)},
		{Slug: "c", Kind: AccessRead, At: imp.Add(-time.Minute)},
		{Slug: "d", Kind: AccessWrite},
	}, imp, false)
	if len(env.events["a"]) != 1 || len(env.events["b"]) != 0 || len(env.events["c"]) != 0 || len(env.events["d"]) != 0 {
		t.Errorf("events %v", env.events)
	}
}

func TestGoldOptionsValidate(t *testing.T) {
	good := DefaultGold()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []GoldOptions{{Tier: 3, Direct: true, Dates: DatesDB}, {Tier: -1, Direct: true, Dates: DatesDB}, {Direct: true, Dates: "x"}, {Dates: DatesDB}} {
		if bad.Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if n := (GoldOptions{}).normalized(); !n.Direct || n.Dates != DatesDB {
		t.Errorf("zero value normalizes to %+v", n)
	}
	if n := (GoldOptions{Bash: true}).normalized(); n.Direct {
		t.Errorf("bash only must stay bash only: %+v", n)
	}
}

func TestGoldRunFlags(t *testing.T) {
	f := newGoldFixture(t)
	out := filepath.Join(f.root, "out")
	matrix, events := filepath.Join(f.root, "m.tsv"), filepath.Join(f.root, "w.tsv")
	args := []string{"--transcripts", filepath.Join(f.projects, "*.jsonl"), "--legacy-dir", f.mem, "--out", out, "--project", "repo", "--all-prompts=false"}
	if code := Run(append(slices.Clone(args), "--gold-tier", "1", "--gold-bash", "--gold-subagent", "--gold-dates", "both", "--gold-matrix", matrix, "--write-events", events), f.e); code != 0 {
		t.Fatalf("exit %d: %s", code, f.stderr.String())
	}
	for _, w := range []string{"gold tier1, sources direct+bash+subagent", "prompts with gold 5 ", "memory writes found: 4 events on 4 files", "tier2-both", "direct+bash+subagent"} {
		if !strings.Contains(f.stdout.String(), w) {
			t.Errorf("stdout lacks %q:\n%s", w, f.stdout.String())
		}
	}
	for _, leak := range []string{"flake inputs", "kernel config", "turn nine"} {
		for _, name := range []string{matrix, events} {
			data, err := os.ReadFile(name)
			if err != nil || strings.Contains(string(data), leak) {
				t.Errorf("%s: %v or prompt text %q present", name, err, leak)
			}
		}
		if strings.Contains(f.stdout.String(), leak) {
			t.Errorf("stdout leaks %q", leak)
		}
	}
	f.stdout.Reset()
	if code := Run(append(slices.Clone(args), "--gold-tier", "2", "--gold-dates", "db", "--json"), f.e); code != 0 {
		t.Fatalf("exit %d: %s", code, f.stderr.String())
	}
	var sum Summary
	if err := json.Unmarshal(f.stdout.Bytes(), &sum); err != nil || sum.Gold == nil || sum.Gold.Tier != "tier2-db" || sum.Evaluated != 2 {
		t.Errorf("json summary: %v %+v evaluated %d", err, sum.Gold, sum.Evaluated)
	}
	for _, bad := range [][]string{{"--gold-tier", "3"}, {"--gold-dates", "never"}, {"--gold-direct=false"}, {"--import-time", "yesterday"}} {
		f.stderr.Reset()
		if code := Run(append(slices.Clone(args), bad...), f.e); code != 2 {
			t.Errorf("%v: exit %d %q", bad, code, f.stderr.String())
		}
	}
}

func TestCheckNamesAllowsExtraVectorsOnly(t *testing.T) {
	if err := checkNames("queries", []string{"a", "b"}, []string{"a"}, false); err == nil {
		t.Error("extra vectors accepted without the flag")
	}
	if err := checkNames("queries", []string{"a", "b"}, []string{"a"}, true); err != nil {
		t.Errorf("extra vectors refused with the flag: %v", err)
	}
	if err := checkNames("queries", []string{"a"}, []string{"a", "b"}, true); err == nil {
		t.Error("missing vectors accepted")
	}
}

func TestSubagentScanReadsBashAndWrites(t *testing.T) {
	dir := t.TempDir()
	mem := "/home/u/.claude/projects/-home-u-repo/memory"
	lg := Legacy{Dirs: []string{mem}, Indexes: map[string]bool{"MEMORY": true}}
	sub := &tx{t: t}
	sub.add(stamped(toolUse("Read", filepath.Join(mem, "a.md"), true), goldAt(1)))
	sub.add(stamped(toolCall("Bash", map[string]any{"command": "rg -n x " + mem + "/b.md && rg -n y " + mem}, goldAt(2)), goldAt(2)))
	sub.add(stamped(toolUse("Write", filepath.Join(mem, "c.md"), true), goldAt(3)))
	sub.add(stamped(toolUse("Read", filepath.Join(mem, "MEMORY.md"), true), goldAt(4)))
	sub.add(stamped(toolCall("Bash", map[string]any{"command": "python3 -c 'print(1)' # " + mem + "/d.md"}, goldAt(5)), goldAt(5)))
	sub.add(stamped(toolUse("Read", filepath.Join(mem, "late.md"), true), goldAt(500)))
	writeFiles(t, dir, map[string]string{"agent-1.jsonl": sub.String(), "workflows/w/agent-2.jsonl": sub.String(), "note.txt": "x"})
	until := time.Date(2026, 11, 6, 10, 30, 0, 0, time.UTC)
	got, err := SubagentScan(dir, "sess", lg, "/home/u", until)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 2 || len(got.Uses) != 10 {
		t.Fatalf("files %d uses %d", got.Files, len(got.Uses))
	}
	var seen []string
	for _, a := range got.Access[:6] {
		seen = append(seen, fmt.Sprintf("%d:%s:%s:%v", a.Kind, a.Slug, a.Tool, a.Index))
		if !a.Sub || a.Session != "sess" || a.At.IsZero() {
			t.Errorf("access %+v", a)
		}
	}
	want := []string{"0:a:Read:false", "1:b:Bash:rg:false", "2:c:Write:false", "0:MEMORY:Read:true"}
	if !slices.Equal(seen[:4], want) {
		t.Errorf("accesses %v, want %v", seen, want)
	}
	if got.Gold.BashCalls != 4 || got.Gold.BashNoEffect != 2 || got.Gold.BashScript != 2 {
		t.Errorf("gold counts %+v", got.Gold)
	}
	missing, err := SubagentScan(filepath.Join(dir, "none"), "s", lg, "", until)
	if err != nil || missing.Files != 0 || len(missing.Access) != 0 {
		t.Errorf("missing dir: %+v %v", missing, err)
	}
}

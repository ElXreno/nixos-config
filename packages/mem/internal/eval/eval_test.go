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
	"mem/internal/store"
)

type tx struct {
	t     *testing.T
	lines []string
}

func (x *tx) add(v map[string]any) *tx {
	data, err := json.Marshal(v)
	if err != nil {
		x.t.Fatal(err)
	}
	x.lines = append(x.lines, string(data))
	return x
}

func human(text string) map[string]any {
	return map[string]any{"type": "user", "isSidechain": false, "origin": map[string]any{"kind": "human"}, "promptSource": "typed",
		"message": map[string]any{"role": "user", "content": text}}
}

func userEntry(content any, extra map[string]any) map[string]any {
	m := map[string]any{"type": "user", "isSidechain": false, "message": map[string]any{"role": "user", "content": content}}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func toolUse(name, path string, side bool) map[string]any {
	return map[string]any{"type": "assistant", "isSidechain": side, "message": map[string]any{"role": "assistant",
		"content": []any{map[string]any{"type": "tool_use", "id": "t", "name": name, "input": map[string]any{"file_path": path}}}}}
}

func toolResult() map[string]any {
	return map[string]any{"type": "user", "isSidechain": false, "toolUseResult": map[string]any{"type": "text"},
		"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t", "content": "x"}}}}
}

func assistantText(s string) map[string]any {
	return map[string]any{"type": "assistant", "isSidechain": false, "message": map[string]any{"role": "assistant",
		"content": []any{map[string]any{"type": "text", "text": s}}}}
}

func queued(text, kind string) map[string]any {
	return map[string]any{"type": "attachment", "isSidechain": false, "attachment": map[string]any{"type": "queued_command",
		"commandMode": "prompt", "prompt": text, "origin": map[string]any{"kind": kind}}}
}

func (x *tx) String() string { return strings.Join(x.lines, "\n") + "\n" }

const longMarker = "ZZMARKERZZ"

func sessionWithOrigin(t *testing.T, mem, other string) string {
	x := &tx{t: t}
	x.add(map[string]any{"type": "permission-mode", "permissionMode": "default"})
	x.add(human("Can I git push the agent-guard commits now?"))
	x.add(assistantText("Checking memory first."))
	x.add(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), false))
	x.add(toolResult())
	x.add(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), false))
	x.add(toolResult())
	x.add(userEntry("<task-notification>done</task-notification>", map[string]any{"origin": map[string]any{"kind": "task-notification"}, "promptSource": "system"}))
	x.add(toolUse("Read", filepath.Join(mem, "reference_bs3pro.md"), false))
	x.add(human("What does the 0xDF command do to the BS3 Pro cooler firmware, and is it safe? " + longMarker))
	x.add(toolUse("Read", filepath.Join(mem, "MEMORY.md"), false))
	x.add(toolUse("Read", filepath.Join(mem, "reference_bs3pro.md"), false))
	x.add(toolUse("Edit", filepath.Join(mem, "reference_bs3pro.md"), false))
	x.add(toolUse("Read", filepath.Join(other, "feedback_x.md"), false))
	x.add(toolUse("Read", filepath.Join(mem, "reference_deleted.md"), false))
	x.add(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), true))
	x.add(assistantText("We were discussing the zfs readahead batch patches and their review."))
	x.add(human("ok?"))
	x.add(userEntry("This session is being continued from a previous conversation.", map[string]any{"isCompactSummary": true}))
	x.add(userEntry([]any{map[string]any{"type": "text", "text": "skill body"}}, map[string]any{"isMeta": true}))
	x.add(toolUse("Read", filepath.Join(mem, "project_saga.md"), false))
	x.add(userEntry("<command-name>/compact</command-name>", nil))
	x.add(toolUse("Read", filepath.Join(mem, "reference_bs3pro.md"), false))
	x.add(userEntry([]any{map[string]any{"type": "text", "text": "[Request interrupted by user]"}}, nil))
	x.add(userEntry("plain text from the sdk", map[string]any{"promptSource": "sdk"}))
	x.add(queued("also check the no push rule again", "human"))
	x.add(toolUse("Read", "~/"+filepath.Join(strings.TrimPrefix(mem, "/home/u/"), "feedback_no_push.md"), false))
	x.add(queued("peer chatter", "peer"))
	x.add(human("/investigate why it broke"))
	x.add(human("/home/u/file broke, why?"))
	x.add(userEntry([]any{map[string]any{"type": "text", "text": "look at this screenshot of bs3pro"}, map[string]any{"type": "image"}},
		map[string]any{"origin": map[string]any{"kind": "human"}}))
	return x.String()
}

func sessionWithoutOrigin(t *testing.T, mem string) string {
	x := &tx{t: t}
	x.add(userEntry("bs3pro 0xdf question", nil))
	x.add(toolUse("Read", filepath.Join(mem, "reference_bs3pro.md"), false))
	x.add(userEntry("<command-name>/memory</command-name>", nil))
	x.add(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), false))
	x.add(userEntry("from a script", map[string]any{"promptSource": "sdk"}))
	x.add(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), false))
	return x.String() + "{\"type\":\"user\", not json\n"
}

func TestParseTranscript(t *testing.T) {
	mem := "/home/u/.claude/projects/-home-u-repo/memory"
	other := "/home/u/.claude/projects/-home-u-other/memory"
	lg := Legacy{Dirs: []string{mem}, Indexes: map[string]bool{"MEMORY": true}}
	opt := ParseOptions{MinChars: 40, Tail: 30, Home: "/home/u"}
	prompts, c, err := ParseTranscript(strings.NewReader(sessionWithOrigin(t, mem, other)), "a", lg, opt)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		turn         int
		gold, wrote  string
		queued, tail bool
		index        int
	}
	var got []row
	for _, p := range prompts {
		got = append(got, row{p.Turn, strings.Join(p.Gold, ","), strings.Join(p.Written, ","), p.Queued, p.ShortTail, p.IndexReads})
	}
	want := []row{
		{turn: 1, gold: "feedback_no_push"},
		{turn: 3, gold: "reference_bs3pro,reference_deleted", wrote: "reference_bs3pro", index: 1},
		{turn: 4, gold: "project_saga", tail: true},
		{turn: 8, gold: "feedback_no_push", queued: true, tail: true},
		{turn: 11, tail: true},
		{turn: 12, tail: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("prompts\n got %+v\nwant %+v", got, want)
	}
	if q := prompts[2].Query; q != "ok?\natch patches and their review." {
		t.Errorf("short prompt query %q", q)
	}
	if prompts[5].Text != "look at this screenshot of bs3pro" {
		t.Errorf("array prompt text %q", prompts[5].Text)
	}
	wantCounts := Counts{Files: 1, Lines: 31, Boundaries: 12, Genuine: 6, Queued: 1, NotHuman: 5, Slash: 1, Sidechain: 1,
		MemoryReads: 8, IndexReads: 1, OtherMemReads: 1, OrphanReads: 2, PromptsRead: 4, OtherUses: 2}
	if c != wantCounts {
		t.Errorf("counts\n got %+v\nwant %+v", c, wantCounts)
	}

	prompts, c, err = ParseTranscript(strings.NewReader(sessionWithoutOrigin(t, mem)), "b", lg, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || !slices.Equal(prompts[0].Gold, []string{"reference_bs3pro"}) || !prompts[0].ShortTail && prompts[0].Query != prompts[0].Text {
		t.Fatalf("no-origin prompts %+v", prompts)
	}
	if c.Tagged != 1 || c.NotHuman != 1 || c.BadLines != 1 || c.OrphanReads != 2 {
		t.Errorf("no-origin counts %+v", c)
	}
}

func TestScore(t *testing.T) {
	hits := func(slugs ...string) []rankedHit {
		var out []rankedHit
		for _, s := range slugs {
			out = append(out, rankedHit{slug: s})
		}
		return out
	}
	cases := []struct {
		name               string
		hits               []rankedHit
		gold               []string
		r4, r8, h4, h8, rr float64
		best               int
	}{
		{"top hit", hits("a", "x"), []string{"a"}, 1, 1, 1, 1, 1, 1},
		{"chunks of one file count once", hits("x", "a", "a", "a", "b"), []string{"a", "b"}, 0.5, 1, 1, 1, 0.5, 2},
		{"rank six", hits("x", "x", "x", "x", "x", "a"), []string{"a"}, 0, 1, 0, 1, 1.0 / 6, 6},
		{"miss", hits("x", "y"), []string{"a"}, 0, 0, 0, 0, 0, 0},
		{"no gold", hits("a"), nil, 0, 0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		sc := scoreOf(c.hits, c.gold)
		want := score{c.r4, c.r8, c.h4, c.h8, c.rr, c.best}
		if sc != want {
			t.Errorf("%s: %+v, want %+v", c.name, sc, want)
		}
	}
	if q := quantiles([]float64{5, 1, 3, 2, 4}); q != (Quantiles{N: 5, P10: 1, P50: 3, P90: 5}) {
		t.Errorf("quantiles %+v", q)
	}
}

func TestHookStats(t *testing.T) {
	keySlug := map[string]string{"a": "a", "b": "b", "c": "c"}
	qs := []*query{
		{gold: []string{"a", "b"}},
		{gold: []string{"c"}},
		{gold: []string{"a"}},
		{},
	}
	outs := []outcome{
		{keys: []string{"a", "x"}, cut: 1, chars: 900},
		{},
		{keys: []string{"a"}, chars: 300},
		{keys: []string{"y"}, chars: 400},
	}
	got := hookStats(SetGold, qs, outs, keySlug)
	want := HookStats{Set: SetGold, Prompts: 4, Injecting: 3, InjectRate: 0.75, Records: 4, Cut: 1, MeanRecords: 1, MeanChars: 400,
		MeanCharsInjecting: 1600.0 / 3, Recall: 0.5, Hit: 2.0 / 3, GoldInjected: 2, Precision: 0.5}
	if got != want {
		t.Errorf("stats\n got %+v\nwant %+v", got, want)
	}
	if neg := hookStats(SetNegative, qs[3:], outs[3:], keySlug); neg.InjectRate != 1 || neg.Recall != 0 || neg.Precision != 0 {
		t.Errorf("negative stats %+v", neg)
	}
}

func TestSampleNegatives(t *testing.T) {
	var pool []Prompt
	for i := range 50 {
		pool = append(pool, Prompt{Session: "s" + string(rune('a'+i%5)), Turn: i})
	}
	a := sampleNegatives(pool, 10, 7)
	b := sampleNegatives(slices.Concat(pool[25:], pool[:25]), 10, 7)
	if len(a) != 10 || !slices.EqualFunc(a, b, func(x, y Prompt) bool { return x.Session == y.Session && x.Turn == y.Turn }) {
		t.Errorf("sample depends on input order: %v vs %v", a, b)
	}
	if c := sampleNegatives(pool, 10, 8); slices.EqualFunc(a, c, func(x, y Prompt) bool { return x.Turn == y.Turn }) {
		t.Error("another seed gave the same sample")
	}
	if all := sampleNegatives(pool, 500, 7); len(all) != len(pool) {
		t.Errorf("oversized sample %d, want %d", len(all), len(pool))
	}
	grown := append(slices.Clone(pool), Prompt{Session: "new", Turn: 999})
	kept := 0
	for _, p := range sampleNegatives(grown, 10, 7) {
		if slices.ContainsFunc(a, func(x Prompt) bool { return x.Session == p.Session && x.Turn == p.Turn }) {
			kept++
		}
	}
	if kept < 9 {
		t.Errorf("one new prompt replaced %d sampled prompts", 10-kept)
	}
}

func TestJudgeCase(t *testing.T) {
	inj := recall.Injection{Packed: recall.Packed{Text: "x", Keys: []string{"k1", "k2"}}}
	q := &query{inj: inj}
	for _, c := range []struct {
		c    Case
		pass bool
	}{
		{Case{Name: "want", Prompt: "p", Want: []string{"k2"}}, true},
		{Case{Name: "want all", Prompt: "p", Want: []string{"k1", "k3"}}, false},
		{Case{Name: "none", Prompt: "p", None: true}, false},
	} {
		if got := judgeCase(c.c, q); got.Pass != c.pass {
			t.Errorf("%s: pass %v, want %v", c.c.Name, got.Pass, c.pass)
		}
	}
	if got := judgeCase(Case{Name: "none", Prompt: "p", None: true}, &query{}); !got.Pass || got.Injected == nil {
		t.Errorf("empty injection: %+v", got)
	}
}

func TestFrontier(t *testing.T) {
	row := func(r, f float64) SweepRow {
		return SweepRow{Gold: HookStats{Recall: r}, Negative: HookStats{InjectRate: f}}
	}
	rows := []SweepRow{row(0.1, 0.05), row(0.2, 0.1), row(0.15, 0.2), row(0.22, 0.5), row(0.05, 0.0)}
	markFrontier(rows)
	var pareto []bool
	knee := -1
	for i, r := range rows {
		pareto = append(pareto, r.Pareto)
		if r.Knee {
			knee = i
		}
	}
	if !slices.Equal(pareto, []bool{true, true, false, true, true}) || knee != 1 {
		t.Errorf("pareto %v knee %d", pareto, knee)
	}
}

func TestCrossFit(t *testing.T) {
	row := func(cover float64, r0, f0, r1, f1 float64) SweepRow {
		return SweepRow{Gate: recall.Gate{MinCover: cover}, Folds: [2]Fold{{Recall: r0, FalseRate: f0}, {Recall: r1, FalseRate: f1}},
			Gold: HookStats{Recall: (r0 + r1) / 2}, Negative: HookStats{InjectRate: (f0 + f1) / 2}}
	}
	rows := []SweepRow{
		row(0.1, 0.30, 0.90, 0.30, 0.90),
		row(0.2, 0.25, 0.20, 0.10, 0.40),
		row(0.3, 0.15, 0.25, 0.20, 0.10),
		row(0.4, 0.05, 0.05, 0.05, 0.05),
	}
	markFrontier(rows)
	var k0, k1 []float64
	for _, r := range rows {
		if r.FoldKnee[0] {
			k0 = append(k0, r.Gate.MinCover)
		}
		if r.FoldKnee[1] {
			k1 = append(k1, r.Gate.MinCover)
		}
	}
	if !slices.Equal(k0, []float64{0.2}) || !slices.Equal(k1, []float64{0.3}) {
		t.Fatalf("fold knees %v %v, want 0.2 on fold 0 and 0.3 on fold 1", k0, k1)
	}
	cf := crossFit(rows)
	if len(cf) != 2 {
		t.Fatalf("cross-fit %+v", cf)
	}
	a, b := cf[0], cf[1]
	if a.PickedOn != 0 || a.Gate.MinCover != 0.2 || a.HeldOut != rows[1].Folds[1] || a.HeldKnee.MinCover != 0.3 || a.HeldAtFIR != 0.20 {
		t.Errorf("pick on fold 0: %+v", a)
	}
	if b.PickedOn != 1 || b.Gate.MinCover != 0.3 || b.HeldOut != rows[2].Folds[0] || b.HeldKnee.MinCover != 0.2 || b.HeldAtFIR != 0.25 {
		t.Errorf("pick on fold 1: %+v", b)
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func saga() string {
	var b strings.Builder
	b.WriteString("---\nname: project_saga\ndescription: ZFS investigation\nmetadata:\n  type: project\n---\n# Saga\n\n")
	for i := 1; i <= 12; i++ {
		b.WriteString("- 2026-10-0" + string(rune('0'+i%10)) + ": zfs readahead batch review round, " + strings.Repeat("numbers and notes ", 12) + "\n")
	}
	return b.String()
}

type evalFixture struct {
	root, home, mem, other, projects string
	e                                env.Env
	stdout, stderr                   *bytes.Buffer
}

func newEvalFixture(t *testing.T) *evalFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	mem := filepath.Join(home, ".claude", "projects", "-home-u-repo", "memory")
	other := filepath.Join(home, ".claude", "projects", "-home-u-other", "memory")
	writeFiles(t, mem, map[string]string{
		"MEMORY.md":           "- feedback_no_push · reference_bs3pro\n",
		"feedback_no_push.md": "---\nname: feedback_no_push\ndescription: No git push by default\ntype: feedback\n---\nDo not git push by default; after a commit wait for explicit authorization.\n",
		"reference_bs3pro.md": "---\nname: BS3 Pro\ndescription: Flydigi BS3 Pro cooler protocol\nmetadata:\n  type: reference\n---\nNever send 0xDF to the BS3 Pro cooler: it bricks the firmware.\n",
		"project_saga.md":     saga(),
	})
	projects := filepath.Join(home, ".claude", "projects", "-home-u-repo")
	writeFiles(t, projects, map[string]string{
		"a.jsonl":                     strings.ReplaceAll(sessionWithOrigin(t, "/home/u/.claude/projects/-home-u-repo/memory", other), "/home/u", home),
		"b.jsonl":                     sessionWithoutOrigin(t, mem),
		"a/subagents/agent-1.jsonl":   sessionWithoutOrigin(t, mem),
		"a/tool-results/ignored.json": "{}",
	})
	vars := map[string]string{"HOME": home}
	x := &evalFixture{root: root, home: home, mem: mem, other: other, projects: projects, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	x.e = env.Env{
		Stdin: strings.NewReader(""), Stdout: x.stdout, Stderr: x.stderr,
		Getenv:   func(k string) string { return vars[k] },
		Now:      func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) },
		Hostname: func() (string, error) { return "smok", nil },
	}
	return x
}

func (x *evalFixture) args(extra ...string) []string {
	return append([]string{"--transcripts", filepath.Join(x.projects, "*.jsonl"), "--legacy-dir", x.mem, "--project", "repo"}, extra...)
}

func TestEvaluate(t *testing.T) {
	fx := newEvalFixture(t)
	root, home, mem, projects, e := fx.root, fx.home, fx.mem, fx.projects, fx.e
	stdout, stderr := fx.stdout, fx.stderr
	if files, err := expandGlob(filepath.Join(projects, "*", "*", "*.jsonl"), home); err != nil || len(files) != 0 {
		t.Fatalf("subagent transcripts not filtered: %v %v", files, err)
	}
	out := filepath.Join(root, "out")
	cases := filepath.Join(root, "cases.json")
	writeFiles(t, root, map[string]string{
		"cases.json": `[{"name": "cooler", "prompt": "What does the 0xDF command do to the BS3 Pro cooler?", "want": ["reference_bs3pro"]},
			{"name": "unrelated", "prompt": "write a haiku about autumn leaves falling in the park", "none": true}]`,
		"bad-cases.json": `[{"name": "cooler", "prompt": "What does the 0xDF command do to the BS3 Pro cooler?", "want": ["feedback_no_push"]}]`,
		"odd-cases.json": `[{"name": "both", "prompt": "x", "want": ["a"], "none": true}]`,
	})
	sweepPath := filepath.Join(root, "sweep.tsv")
	args := []string{"--transcripts", filepath.Join(projects, "*.jsonl"), "--legacy-dir", mem, "--out", out, "--project", "repo", "--cases", cases, "--sweep", sweepPath}
	if code := Run(args, e); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, w := range []string{"transcripts 2", "genuine prompts 7 (1 queued mid-turn)", "prompts with gold 5", "missing from db 1",
		"stored as log 1", "written in the same turn 1", "words/all      read                    5   1.000   1.000",
		"hook path (fused, no log, pool 12, cover >= 0.40, shared >= 4, topic >= 1, lead shared >= 2, at most 4 records, 2500 chars)",
		"negative set: all 2 genuine prompts without memory use", "cases 2 of 2 pass", "PASS cooler: want reference_bs3pro",
		"PASS unrelated: want nothing, injected nothing", "sweep: " + fmt.Sprint(len(SweepGrid())) + " settings", "knee over both folds: cover >= "} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout lacks %q:\n%s", w, stdout.String())
		}
	}
	for _, leak := range []string{longMarker, "git push the agent-guard", "screenshot"} {
		if strings.Contains(stdout.String(), leak) {
			t.Errorf("stdout leaks prompt text %q", leak)
		}
	}
	data, err := os.ReadFile(filepath.Join(out, "queries.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(rows) != 6 || strings.Contains(string(data), longMarker) {
		t.Fatalf("queries.tsv rows %d or full prompt text present:\n%s", len(rows), data)
	}
	head := strings.Split(rows[0], "\t")
	if head[2] != "hash" || head[3] != "prompt60" || len(head) != 11+3*len(Runs) {
		t.Fatalf("header %v", head)
	}
	first := strings.Split(rows[1], "\t")
	if first[2] != PromptHash("Can I git push the agent-guard commits now?") || first[3] != "Can I git push the agent-guard commits now?" || first[10] != "feedback_no_push" {
		t.Fatalf("first row %q", first)
	}
	third := strings.Split(rows[2], "\t")
	if n := len([]rune(third[3])); n != PromptPrefix {
		t.Fatalf("prompt prefix has %d chars", n)
	}
	var sum Summary
	data, err = os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil || json.Unmarshal(data, &sum) != nil {
		t.Fatalf("summary.json: %v", err)
	}
	if len(sum.Metrics) != 2*len(Runs) || sum.Evaluated != 5 || sum.Records == 0 {
		t.Fatalf("summary %+v", sum)
	}
	for _, m := range sum.Metrics {
		if m.Run == "words/nolog" && m.Gold == "read" && (m.Prompts != 5 || m.Recall4 != 0.8) {
			t.Errorf("words/nolog %+v, want 4 of 5 prompts recalled (the log-only one cannot be)", m)
		}
		if m.Gold == "read-not-written" && m.Prompts != 4 {
			t.Errorf("%s read-not-written prompts %d, want 4", m.Run, m.Prompts)
		}
		if m.Run == "hook" && m.Gold == "read" && (m.Recall4 != m.Recall8 || m.Recall4 > 0.8) {
			t.Errorf("hook gate %+v: emits at most 4 records and never a log", m)
		}
	}
	if len(sum.Hook) != 3 || sum.Hook[0].Set != SetGold || sum.Hook[0].Prompts != 5 || sum.Hook[1].Set != SetNegative || sum.Hook[1].Prompts != 2 ||
		sum.Hook[2].Set != SetProject || sum.Hook[2].Prompts != 7 || sum.NegativePool != 2 || len(sum.Cases) != 2 {
		t.Fatalf("hook stats %+v cases %+v", sum.Hook, sum.Cases)
	}
	for _, name := range []string{"hook.tsv", "candidates.tsv", "hook-summary.tsv"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || strings.Contains(string(data), longMarker) {
			t.Errorf("%s: %v or full prompt text present", name, err)
		}
	}
	if data, err := os.ReadFile(sweepPath); err != nil || len(strings.Split(strings.TrimSpace(string(data)), "\n")) != 1+len(SweepGrid()) {
		t.Errorf("sweep.tsv: %v", err)
	}
	stderr.Reset()
	if code := Run(append(slices.Clone(args[:8]), "--cases", filepath.Join(root, "bad-cases.json")), e); code != 1 || !strings.Contains(stderr.String(), "1 of 1 cases fail") {
		t.Errorf("failing case: exit %d %q", code, stderr.String())
	}
	stderr.Reset()
	if code := Run(append(slices.Clone(args[:8]), "--cases", filepath.Join(root, "odd-cases.json")), e); code != 1 || !strings.Contains(stderr.String(), "either want keys or none") {
		t.Errorf("malformed case: exit %d %q", code, stderr.String())
	}
	s, err := store.Open(t.Context(), filepath.Join(out, DBName), store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if recs, err := s.List(t.Context(), store.Filter{}); err != nil || len(recs) < 4 {
		t.Fatalf("eval db records %d %v", len(recs), err)
	}
	if code := Run(append(args, "--json"), e); code != 0 {
		t.Fatalf("rerun exit %d: %s", code, stderr.String())
	}
	usage := []struct {
		args []string
		want string
	}{
		{nil, "--legacy-dir and --out are required"},
		{[]string{"--legacy-dir", mem, "--out", out, "--depth", "4"}, "--depth must be at least 8"},
		{[]string{"--legacy-dir", mem, "--out", out, "x"}, "unexpected argument"},
		{[]string{"--legacy-dir", mem, "--out", out, "--until", "yesterday"}, "--until"},
		{[]string{"--legacy-dir", mem, "--out", out, "--gate-shared", "-1"}, "the rest non-negative"},
	}
	for _, u := range usage {
		stderr.Reset()
		if code := Run(u.args, e); code != 2 || !strings.Contains(stderr.String(), u.want) {
			t.Errorf("Run(%q) = %d %q", u.args, code, stderr.String())
		}
	}
	stderr.Reset()
	if code := Run([]string{"--legacy-dir", mem, "--out", out, "--transcripts", filepath.Join(root, "none", "*.jsonl")}, e); code != 1 ||
		!strings.Contains(stderr.String(), "no transcripts") {
		t.Errorf("empty glob = %d %q", code, stderr.String())
	}
}

func toolCall(name string, input map[string]any, stamp string) map[string]any {
	return map[string]any{"type": "assistant", "isSidechain": false, "timestamp": stamp, "message": map[string]any{"role": "assistant",
		"content": []any{map[string]any{"type": "tool_use", "id": "t", "name": name, "input": input}}}}
}

func stamped(v map[string]any, stamp string) map[string]any {
	v["timestamp"] = stamp
	return v
}

func TestNegativePoolSkipsMemoryUse(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	projects := filepath.Join(home, ".claude", "projects", "-home-u-repo")
	mem := filepath.Join(projects, "memory")
	other := filepath.Join(home, ".claude", "projects", "-home-u-other", "memory")
	writeFiles(t, mem, map[string]string{
		"MEMORY.md":           "- feedback_no_push\n",
		"feedback_no_push.md": "---\nname: feedback_no_push\ndescription: No git push by default\ntype: feedback\n---\nDo not git push by default.\n",
	})
	at := func(m int) string { return fmt.Sprintf("2026-11-06T10:%02d:00.000Z", m) }
	x := &tx{t: t}
	x.add(stamped(human("gold turn: may I git push the branch now?"), at(0)))
	x.add(toolCall("Read", map[string]any{"file_path": filepath.Join(mem, "feedback_no_push.md")}, at(1)))
	x.add(stamped(human("clean turn one about the flake inputs"), at(2)))
	x.add(stamped(assistantText("Nothing to read."), at(3)))
	x.add(stamped(human("bash turn about the push rule"), at(4)))
	x.add(toolCall("Bash", map[string]any{"command": "cat ~/.claude/projects/-home-u-repo/memory/feedback_no_push.md"}, at(5)))
	x.add(stamped(human("grep turn about the push rule"), at(6)))
	x.add(toolCall("Grep", map[string]any{"pattern": "push", "path": mem}, at(7)))
	x.add(stamped(human("edit turn that records a new rule"), at(8)))
	x.add(toolCall("Edit", map[string]any{"file_path": filepath.Join(mem, "feedback_no_push.md"), "old_string": "a", "new_string": "b"}, at(9)))
	x.add(stamped(human("turn that reads another project's memory"), at(10)))
	x.add(toolCall("Read", map[string]any{"file_path": filepath.Join(other, "feedback_x.md")}, at(11)))
	x.add(stamped(human("agent turn that delegates the lookup"), at(12)))
	x.add(toolCall("Agent", map[string]any{"prompt": "summarize the build setup"}, at(13)))
	x.add(stamped(human("turn that runs the mem tool"), at(16)))
	x.add(toolCall("Bash", map[string]any{"command": "mem search git push"}, at(17)))
	x.add(stamped(human("clean turn two about the kernel config"), at(18)))
	x.add(stamped(human("workflow turn that fans out"), at(20)))
	x.add(toolCall("Workflow", map[string]any{"script": "run.js"}, at(21)))
	sub := &tx{t: t}
	sub.add(map[string]any{"type": "user", "isSidechain": true, "timestamp": at(13), "message": map[string]any{"role": "user", "content": "find it"}})
	sub.add(stamped(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), true), at(14)))
	wf := &tx{t: t}
	wf.add(toolCall("Bash", map[string]any{"command": "rg -n push " + other}, at(22)))
	writeFiles(t, projects, map[string]string{
		"n.jsonl":                   x.String(),
		"n/subagents/agent-1.jsonl": sub.String(),
		"n/subagents/workflows/wf_1/agent-2.jsonl": wf.String(),
	})
	vars := map[string]string{"HOME": home}
	var stdout, stderr bytes.Buffer
	e := env.Env{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, Getenv: func(k string) string { return vars[k] },
		Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }, Hostname: func() (string, error) { return "smok", nil }}
	out := filepath.Join(root, "out")
	if code := Run([]string{"--transcripts", filepath.Join(projects, "*.jsonl"), "--legacy-dir", mem, "--out", out, "--project", "repo"}, e); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var sum Summary
	data, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil || json.Unmarshal(data, &sum) != nil {
		t.Fatalf("summary.json: %v", err)
	}
	if sum.Evaluated != 1 || sum.NegativePool != 2 || sum.Counts.Genuine != 10 {
		t.Errorf("gold prompts %d, negative pool %d, genuine %d: want 1, the 2 clean turns, 10", sum.Evaluated, sum.NegativePool, sum.Counts.Genuine)
	}
	if c := sum.Counts; c.SubagentFiles != 2 || c.SubagentUses != 2 || c.PromptsUsing != 7 || c.OtherUses != 5 {
		t.Errorf("counts %+v: want 2 subagent files with 2 uses, 5 main-session uses, 7 prompts left out", c)
	}
	hook, err := os.ReadFile(filepath.Join(out, "hook.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	var negTurns []string
	for _, row := range strings.Split(strings.TrimSpace(string(hook)), "\n")[1:] {
		if f := strings.Split(row, "\t"); f[0] == SetNegative {
			negTurns = append(negTurns, f[2])
		}
	}
	slices.Sort(negTurns)
	if !slices.Equal(negTurns, []string{"2", "9"}) {
		t.Errorf("negative turns %v, want the clean turns 2 and 9", negTurns)
	}
}

func TestUntil(t *testing.T) {
	mem := "/home/u/.claude/projects/-home-u-repo/memory"
	lg := Legacy{Dirs: []string{mem}, Indexes: map[string]bool{}}
	x := &tx{t: t}
	early, lateMsg := human("Can I git push the agent-guard commits now?"), human("What does the 0xDF command do to the BS3 Pro cooler firmware?")
	early["timestamp"], lateMsg["timestamp"] = "2026-11-07T00:10:00.000Z", "2026-11-07T00:30:00.500Z"
	read := toolUse("Read", filepath.Join(mem, "reference_bs3pro.md"), false)
	read["timestamp"] = "2026-11-07T00:31:00Z"
	x.add(early).add(toolUse("Read", filepath.Join(mem, "feedback_no_push.md"), false)).add(lateMsg).add(read)
	until := time.Date(2026, 11, 7, 0, 20, 0, 0, time.UTC)
	prompts, c, err := ParseTranscript(strings.NewReader(x.String()), "u", lg, ParseOptions{Until: until})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || !slices.Equal(prompts[0].Gold, []string{"feedback_no_push"}) || c.Late != 2 {
		t.Errorf("prompts %+v late %d", prompts, c.Late)
	}
	if prompts, _, _ := ParseTranscript(strings.NewReader(x.String()), "u", lg, ParseOptions{}); len(prompts) != 2 {
		t.Errorf("no cutoff: %d prompts", len(prompts))
	}
}

func TestParseTranscriptTailSkipsAPIErrors(t *testing.T) {
	x := &tx{t: t}
	x.add(human("Can I git push the agent-guard commits now, or is the rule still open?"))
	x.add(assistantText("The push rule is still open, ask first."))
	apiErr := assistantText("API Error: 529 overloaded_error")
	apiErr["isApiErrorMessage"] = true
	x.add(apiErr)
	x.add(human("ok?"))
	prompts, _, err := ParseTranscript(strings.NewReader(x.String()), "a", Legacy{Indexes: map[string]bool{}}, ParseOptions{MinChars: DefaultMinChars, Tail: DefaultTail, MaxQuery: DefaultMaxQuery})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 {
		t.Fatalf("prompts %d", len(prompts))
	}
	if want := "ok?\nThe push rule is still open, ask first."; prompts[1].Query != want {
		t.Errorf("query %q, the hook ignores API error messages and sends %q", prompts[1].Query, want)
	}
}

func TestParseTranscriptTailBeyondHookScanWindow(t *testing.T) {
	build := func(fillerBytes int) string {
		x := &tx{t: t}
		x.add(human("Can I git push the agent-guard commits now, or is the rule still open?"))
		x.add(assistantText("The push rule is still open, ask first."))
		pad := strings.Repeat("p", 100000)
		for n := 0; n < fillerBytes; n += len(pad) {
			x.add(toolUse("Bash", "/x/"+pad, false))
		}
		x.add(human("ok?"))
		return x.String()
	}
	opt := ParseOptions{MinChars: DefaultMinChars, Tail: DefaultTail, MaxQuery: DefaultMaxQuery}
	near, _, err := ParseTranscript(strings.NewReader(build(HookTranscriptScan/2)), "a", Legacy{Indexes: map[string]bool{}}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ok?\nThe push rule is still open, ask first."; near[1].Query != want {
		t.Errorf("tail inside the window: %q", near[1].Query)
	}
	far, _, err := ParseTranscript(strings.NewReader(build(HookTranscriptScan+HookTranscriptScan/4)), "a", Legacy{Indexes: map[string]bool{}}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if far[1].Query != "ok?" || far[1].ShortTail {
		t.Errorf("the hook scans only the last %d bytes of the transcript and finds no tail; eval query %q", HookTranscriptScan, far[1].Query)
	}
}

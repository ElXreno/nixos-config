package eval

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestScanShellPatternOperandIsNoPath(t *testing.T) {
	m := "~/.claude/projects/-home-u-repo/memory"
	cases := []struct {
		name string
		cmd  string
		want []string
	}{
		{"rg pattern after cd", "cd " + m + " && rg -l 'b.md' .", nil},
		{"rg pattern then file", "cd " + m + " && rg -n 'b.md' a.md", []string{"R:rg:a"}},
		{"grep pattern then file", "cd " + m + " && grep -n 'b.md' a.md", []string{"R:grep:a"}},
		{"grep -e pattern then file", "cd " + m + " && grep -e 'b.md' a.md", []string{"R:grep:a"}},
		{"rg -e pattern then file", "cd " + m + " && rg -e 'b.md' a.md", []string{"R:rg:a"}},
		{"rg option argument", "cd " + m + " && rg -g 'b.md' pat a.md", []string{"R:rg:a"}},
		{"rg context argument", "cd " + m + " && rg -A 3 'c.md' a.md", []string{"R:rg:a"}},
		{"sed script", "cd " + m + " && sed 's/x/b.md/' a.md", []string{"R:sed:a"}},
		{"sed -e script", "cd " + m + " && sed -n -e '/b.md/p' a.md", []string{"R:sed:a"}},
		{"awk program", "cd " + m + " && awk '/b.md/' a.md", []string{"R:awk:a"}},
		{"head count", "cd " + m + " && head -n 5 a.md", []string{"R:head:a"}},
		{"absolute pattern stays a pattern", "rg -n " + m + "/b.md " + m + "/a.md", []string{"R:rg:a"}},
		{"rg -f patterns", "rg -f " + m + "/b.md " + m + "/a.md", []string{"R:rg:b", "R:rg:a"}},
		{"cat is not a pattern command", "cat " + m + "/a.md " + m + "/b.md", []string{"R:cat:a", "R:cat:b"}},
	}
	for _, c := range cases {
		got := render(ScanShell(c.cmd, shHome, shAccept))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %q\n got %v\nwant %v", c.name, c.cmd, got, c.want)
		}
	}
}

func TestScanShellLoopOverLiteralNames(t *testing.T) {
	m := "~/.claude/projects/-home-u-repo/memory"
	cases := []struct {
		name string
		cmd  string
		want []string
	}{
		{"cat in loop", "for f in a b; do cat " + m + "/$f.md; done", []string{"R:cat:a", "R:cat:b"}},
		{"cd then loop", "cd " + m + " && for f in a b; do cat $f.md; done", []string{"R:cat:a", "R:cat:b"}},
		{"braced variable", "for f in a b; do cat \"$HOME/.claude/projects/-home-u-repo/memory/${f}.md\"; done", []string{"R:cat:a", "R:cat:b"}},
		{"names with suffix", "cd " + m + " && for f in a.md b.md; do sed -n 1,5p $f; done", []string{"R:sed:a", "R:sed:b"}},
		{"sed -i in loop", "M=" + m + "; for h in a b c; do sed -i 's/x/y/' $M/$h.md; done", []string{"W:sed-i:a", "W:sed-i:b", "W:sed-i:c"}},
		{"append in loop", "for f in a b; do echo x >> " + m + "/$f.md; done", []string{"W:redirect:a", "W:redirect:b"}},
		{"copy to elsewhere is no write", "for f in a b; do cp " + m + "/$f.md /tmp/$f.bak; done", nil},
		{"command substitution list stays unknown", "for f in $(ls); do cat " + m + "/$f.md; done", nil},
		{"glob list stays unknown", "cd " + m + " && for f in *.md; do cat $f; done", nil},
		{"two variables", "for x in a b; do for y in c; do cat " + m + "/$x.md " + m + "/$y.md; done; done", []string{"R:cat:a", "R:cat:b", "R:cat:c"}},
		{"unused variable", "for f in a b; do echo $f; done; cat " + m + "/c.md", []string{"R:cat:c"}},
		{"reassigned inside is unknown", "for f in a b; do f=$X; cat " + m + "/$f.md; done", nil},
	}
	for _, c := range cases {
		got := render(ScanShell(c.cmd, shHome, shAccept))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %q\n got %v\nwant %v", c.name, c.cmd, got, c.want)
		}
	}
}

func runLegacy() Legacy {
	return Legacy{Dirs: []string{"/home/u/.claude/projects/-home-u-repo/memory"}, Indexes: map[string]bool{"MEMORY": true}, Slugs: map[string]bool{"a": true, "b": true, "c": true}}
}

func runMem(name string) string {
	return filepath.Join("/home/u/.claude/projects/-home-u-repo/memory", name+".md")
}

func parseRun(t *testing.T, x *tx, opt ParseOptions) Parsed {
	t.Helper()
	opt.Home = "/home/u"
	got, err := ParseTranscriptFull(strings.NewReader(x.String()), "s", runLegacy(), opt)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func edit(slug string, m int) map[string]any {
	return toolCall("Edit", map[string]any{"file_path": runMem(slug), "old_string": "a", "new_string": "b"}, goldAt(m))
}

func read(slug string, m int) map[string]any {
	return stamped(toolUse("Read", runMem(slug), false), goldAt(m))
}

func notification(m int) map[string]any {
	return stamped(userEntry("<task-notification>done</task-notification>", map[string]any{"origin": map[string]any{"kind": "task-notification"}, "promptSource": "system"}), goldAt(m))
}

func TestGoldWritesCoverTheWholeRun(t *testing.T) {
	x := &tx{t: t}
	x.add(stamped(human("first typed prompt that reads rule a and rule b"), goldAt(0)))
	x.add(read("a", 1))
	x.add(read("b", 2))
	x.add(notification(3))
	x.add(edit("a", 4))
	x.add(stamped(queued("second prompt typed while the agent was busy", "human"), goldAt(5)))
	x.add(edit("b", 6))
	x.add(stamped(human("third typed prompt that starts a new run"), goldAt(10)))
	x.add(read("c", 11))
	x.add(edit("c", 12))
	got := parseRun(t, x, ParseOptions{})
	p := got.Prompts
	if len(p) != 3 {
		t.Fatalf("prompts %d: %+v", len(p), p)
	}
	if !slices.Equal(p[0].Gold, []string{"a", "b"}) {
		t.Errorf("first gold %v", p[0].Gold)
	}
	if w := slices.Clone(p[0].WrittenAll); !slices.Equal(sortedCopy(w), []string{"a", "b"}) {
		t.Errorf("a write after a notification and b write after a queued prompt are edits of the first run: %v", p[0].WrittenAll)
	}
	if !slices.Equal(p[1].WrittenAll, []string{"b"}) {
		t.Errorf("queued prompt sees only the writes after it: %v", p[1].WrittenAll)
	}
	if !slices.Equal(p[2].WrittenAll, []string{"c"}) {
		t.Errorf("a typed prompt starts a new run: %v", p[2].WrittenAll)
	}
}

func sortedCopy(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

func TestGoldSubagentAccessFollowsTheSpawnTurn(t *testing.T) {
	x := &tx{t: t}
	x.add(stamped(human("first turn that spawns a background lookup"), goldAt(0)))
	x.add(stamped(human("second turn that starts while the lookup still runs"), goldAt(10)))
	x.add(notification(15))
	x.add(stamped(human("third turn after the lookup finished"), goldAt(20)))
	spawn := time.Date(2026, 11, 6, 10, 1, 0, 0, time.UTC)
	got := parseRun(t, x, ParseOptions{SubAccess: []Access{
		{Spawn: spawn, At: time.Date(2026, 11, 6, 10, 12, 0, 0, time.UTC), Slug: "a", Kind: AccessRead, Tool: "Read", Sub: true},
		{Spawn: spawn, At: time.Date(2026, 11, 6, 10, 16, 0, 0, time.UTC), Slug: "b", Kind: AccessWrite, Tool: "Edit", Sub: true},
		{Spawn: time.Date(2026, 11, 6, 10, 14, 0, 0, time.UTC), At: time.Date(2026, 11, 6, 10, 16, 0, 0, time.UTC), Slug: "c", Kind: AccessRead, Tool: "Read", Sub: true},
	}})
	p := got.Prompts
	if len(p) != 3 {
		t.Fatalf("prompts %d", len(p))
	}
	if !slices.Equal(p[0].SubGold, []string{"a"}) || !slices.Equal(p[0].WrittenAll, []string{"b"}) {
		t.Errorf("the lookup was spawned in the first turn: subgold %v written %v", p[0].SubGold, p[0].WrittenAll)
	}
	if !slices.Equal(p[1].SubGold, []string{"c"}) || len(p[1].WrittenAll) != 0 {
		t.Errorf("second turn only overlapped the first lookup and spawned the third: subgold %v written %v", p[1].SubGold, p[1].WrittenAll)
	}
	if got.Gold.SubCrossTurn != 3 {
		t.Errorf("cross turn accesses %d", got.Gold.SubCrossTurn)
	}
	if got.Gold.SubUnattributed != 0 {
		t.Errorf("unattributed %d", got.Gold.SubUnattributed)
	}
}

func TestGoldSubagentScanRecordsTheSpawnTime(t *testing.T) {
	dir := t.TempDir()
	lg := runLegacy()
	sub := &tx{t: t}
	sub.add(map[string]any{"type": "user", "isSidechain": true, "timestamp": goldAt(1), "message": map[string]any{"role": "user", "content": "find it"}})
	sub.add(stamped(toolUse("Read", runMem("a"), true), goldAt(9)))
	writeFiles(t, dir, map[string]string{"agent-1.jsonl": sub.String()})
	got, err := SubagentScan(dir, "s", lg, "/home/u", time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))
	if err != nil || len(got.Access) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
	if a := got.Access[0]; !a.Spawn.Equal(time.Date(2026, 11, 6, 10, 1, 0, 0, time.UTC)) || !a.At.Equal(time.Date(2026, 11, 6, 10, 9, 0, 0, time.UTC)) {
		t.Errorf("access %+v", a)
	}
}

func TestGoldScriptWritesAreEditsOfTheNamedFiles(t *testing.T) {
	mem := "/home/u/.claude/projects/-home-u-repo/memory"
	x := &tx{t: t}
	call := func(cmd string, m int) map[string]any {
		return toolCall("Bash", map[string]any{"command": cmd}, goldAt(m))
	}
	x.add(stamped(human("turn that reads rules a and b and rewrites a with a script"), goldAt(0)))
	x.add(read("a", 1))
	x.add(read("b", 2))
	x.add(call("python3 - <<'EOF'\nfrom pathlib import Path\np = Path('"+mem+"/a.md')\np.write_text(p.read_text().replace('x', 'y'))\nEOF", 3))
	x.add(stamped(human("turn that reads rule c with a script and changes nothing"), goldAt(10)))
	x.add(read("c", 11))
	x.add(call("python3 -c \"print(open('"+mem+"/c.md').read())\"", 12))
	x.add(stamped(human("turn that renames through a loop with a variable name"), goldAt(20)))
	x.add(call("cd "+mem+" && python3 - <<'EOF'\nimport os\nfor s in ('b',):\n    os.rename(s + '.md', s + '.bak')\nEOF", 21))
	got := parseRun(t, x, ParseOptions{})
	p := got.Prompts
	if len(p) != 3 {
		t.Fatalf("prompts %d", len(p))
	}
	if !slices.Equal(p[0].ScriptWrites, []string{"a"}) || len(p[0].WrittenAll) != 0 {
		t.Errorf("script that writes a: %v %v", p[0].ScriptWrites, p[0].WrittenAll)
	}
	if len(p[1].ScriptWrites) != 0 {
		t.Errorf("read-only script: %v", p[1].ScriptWrites)
	}
	if !slices.Equal(p[2].ScriptWrites, []string{"b"}) {
		t.Errorf("script that renames b: %v", p[2].ScriptWrites)
	}
	plain, with := GoldOptions{Tier: 1, Direct: true, Dates: DatesDB}, GoldOptions{Tier: 1, Direct: true, Dates: DatesDB, Scripts: true}
	if g := plain.pick(p[0], nil).gold; !slices.Equal(g, []string{"a", "b"}) {
		t.Errorf("tier 1 without scripts keeps %v", g)
	}
	if g := with.pick(p[0], nil).gold; !slices.Equal(g, []string{"b"}) {
		t.Errorf("tier 1 with scripts keeps %v", g)
	}
	if with.TierName() != "tier1+scripts" || plain.TierName() != "tier1" {
		t.Errorf("names %s %s", with.TierName(), plain.TierName())
	}
	if got.Gold.BashScript == 0 {
		t.Errorf("counts %+v", got.Gold)
	}
	var ev []string
	for _, w := range got.Writes {
		ev = append(ev, w.Slug+":"+w.Tool)
	}
	if !slices.Equal(ev, []string{"a:Bash:script", "b:Bash:script"}) {
		t.Errorf("write events %v", ev)
	}
}

func TestGoldNegativesSeeTheWholeRun(t *testing.T) {
	x := &tx{t: t}
	x.add(stamped(human("first typed prompt that never touches memory itself"), goldAt(0)))
	x.add(notification(3))
	x.add(edit("a", 4))
	x.add(stamped(human("second typed prompt that is clean"), goldAt(10)))
	x.add(stamped(queued("third prompt typed while the second one runs", "human"), goldAt(11)))
	x.add(read("b", 12))
	x.add(stamped(human("fourth typed prompt after a queued one"), goldAt(20)))
	got := parseRun(t, x, ParseOptions{})
	p := got.Prompts
	if len(p) != 4 {
		t.Fatalf("prompts %d", len(p))
	}
	want := []bool{true, false, false, false}
	for i, w := range want {
		if p[i].TailTouched != w || touched(p[i]) != (i == 0 || i == 2) {
			t.Errorf("prompt %d: tail touched %v, touched %v", i, p[i].TailTouched, touched(p[i]))
		}
	}
}

func TestGoldCompactionDoesNotStartATurn(t *testing.T) {
	x := &tx{t: t}
	x.add(stamped(human("typed prompt that reads rule a and edits it after a compaction"), goldAt(0)))
	x.add(read("a", 1))
	x.add(stamped(userEntry("This session is being continued from a previous conversation", map[string]any{"isCompactSummary": true}), goldAt(2)))
	x.add(edit("a", 3))
	got := parseRun(t, x, ParseOptions{})
	p := got.Prompts
	if len(p) != 1 || !slices.Equal(p[0].Gold, []string{"a"}) || !slices.Equal(p[0].WrittenAll, []string{"a"}) {
		t.Fatalf("prompts %+v", p)
	}
}

func TestGoldInterruptedTurnEndsTheRun(t *testing.T) {
	x := &tx{t: t}
	x.add(stamped(human("typed prompt that reads rule a"), goldAt(0)))
	x.add(read("a", 1))
	x.add(stamped(human("[Request interrupted by user]"), goldAt(2)))
	x.add(edit("a", 3))
	x.add(stamped(human("next typed prompt after the interruption"), goldAt(10)))
	got := parseRun(t, x, ParseOptions{})
	p := got.Prompts
	if len(p) != 2 || !slices.Equal(p[0].Gold, []string{"a"}) || len(p[0].WrittenAll) != 0 || len(p[1].WrittenAll) != 0 {
		t.Fatalf("prompts %+v", p)
	}
}

package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	suspectsLine = regexp.MustCompile(`(?m)^(?:suspects|related subsystems): (.*)$`)
	anomalyLine  = regexp.MustCompile(`ANOMALY code=([0-9A-F]+)`)
)

type simAgent struct {
	stand       *Stand
	angles      []string
	done        map[cell]bool
	seen        map[string]bool
	frontier    []string
	forgetEvery int
	diags       int
}

func (a *simAgent) learn(out string) {
	for _, m := range suspectsLine.FindAllStringSubmatch(out, -1) {
		for _, name := range strings.Split(m[1], ", ") {
			if name != "" && !a.seen[name] {
				a.seen[name] = true
				a.frontier = append(a.frontier, name)
			}
		}
	}
}

func (a *simAgent) next() (cell, bool) {
	for _, sub := range a.frontier {
		for _, angle := range a.angles {
			if c := (cell{sub, angle}); !a.done[c] {
				return c, true
			}
		}
	}
	return cell{}, false
}

func (a *simAgent) run(limit int) bool {
	for range limit {
		c, ok := a.next()
		if !ok {
			out, code := a.stand.Run([]string{"start"})
			if code == 0 {
				return true
			}
			before := len(a.frontier)
			a.learn(out)
			if len(a.frontier) == before {
				return false
			}
			continue
		}
		out, code := a.stand.Run([]string{"diag", c.sub, c.angle})
		if code != 0 {
			return false
		}
		a.diags++
		a.done[c] = true
		a.learn(out)
		if m := anomalyLine.FindStringSubmatch(out); m != nil {
			if _, code := a.stand.Run([]string{"fix", c.sub, m[1]}); code != 0 {
				return false
			}
			if _, code := a.stand.Run([]string{"start"}); code == 0 {
				return true
			}
		}
		if a.forgetEvery > 0 && a.diags%a.forgetEvery == 0 {
			a.done = map[cell]bool{}
			a.seen = map[string]bool{}
			a.frontier = nil
			a.learn(out)
		}
	}
	return false
}

func simulate(t *testing.T, seed uint64, forgetEvery int) (Score, bool) {
	t.Helper()
	var journal bytes.Buffer
	p := DefaultParams(seed)
	agent := &simAgent{
		stand:       NewStand(p, &journal),
		angles:      p.Angles,
		done:        map[cell]bool{},
		seen:        map[string]bool{},
		forgetEvery: forgetEvery,
	}
	solved := agent.run(2000)
	sc, err := ScoreJournal(&journal)
	if err != nil {
		t.Fatal(err)
	}
	return sc, solved
}

func TestAgentWithMemoryNeverRewalks(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		sc, solved := simulate(t, seed, 0)
		if !solved || !sc.Solved {
			t.Fatalf("seed %d: not solved: %+v", seed, sc)
		}
		if sc.RewalkCalls != 0 || sc.RepeatCells != 0 {
			t.Fatalf("seed %d: rewalks %d, repeats %d", seed, sc.RewalkCalls, sc.RepeatCells)
		}
		if sc.Diags < 60 || sc.Fixes != 2 {
			t.Fatalf("seed %d: diags %d, fixes %d", seed, sc.Diags, sc.Fixes)
		}
	}
}

func TestForgetfulAgentIsCounted(t *testing.T) {
	total := 0
	for seed := uint64(1); seed <= 20; seed++ {
		sc, _ := simulate(t, seed, 15)
		if sc.RewalkCalls > sc.Diags || sc.RewalkSubs > sc.RewalkCalls {
			t.Fatalf("seed %d: inconsistent score %+v", seed, sc)
		}
		total += sc.RewalkCalls
	}
	if total == 0 {
		t.Fatal("a forgetful agent produced no rewalks in 20 seeds")
	}
}

func TestReportsAreStable(t *testing.T) {
	p := DefaultParams(7)
	a, b := NewStand(p, nil), NewStand(p, nil)
	startA, _ := a.Run([]string{"start"})
	startB, _ := b.Run([]string{"start"})
	if startA != startB {
		t.Fatal("start differs between two stands of one seed")
	}
	sub := strings.Split(suspectsLine.FindStringSubmatch(startA)[1], ", ")[0]
	first, _ := a.Run([]string{"diag", sub, "net"})
	again, _ := a.Run([]string{"diag", sub, "net"})
	if first != again {
		t.Fatal("a repeated check printed a different report")
	}
	if other, _ := b.Run([]string{"diag", sub, "net"}); other != first {
		t.Fatal("report differs between two stands of one seed")
	}
	if n := strings.Count(first, "\n"); n != p.Lines+3 {
		t.Fatalf("report has %d lines, want %d", n, p.Lines+3)
	}
}

func TestUnknownAndBudget(t *testing.T) {
	var journal bytes.Buffer
	p := DefaultParams(3)
	p.Budget = 2
	stand := NewStand(p, &journal)
	out, _ := stand.Run([]string{"start"})
	sub := strings.Split(suspectsLine.FindStringSubmatch(out)[1], ", ")[0]
	if _, code := stand.Run([]string{"diag", "nosuch", "net"}); code != 2 {
		t.Fatalf("unknown subsystem: code %d", code)
	}
	if _, code := stand.Run([]string{"diag", sub, "bogus"}); code != 2 {
		t.Fatalf("unknown angle: code %d", code)
	}
	stand.Run([]string{"diag", sub, "net"})
	stand.Run([]string{"diag", sub, "disk"})
	if _, code := stand.Run([]string{"diag", sub, "conf"}); code != 3 {
		t.Fatalf("budget: code %d", code)
	}
	sc, err := ScoreJournal(&journal)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Unknown != 1 || sc.Diags != 2 || !sc.Budget || sc.Starts != 1 {
		t.Fatalf("score %+v", sc)
	}
}

func TestThrottleAllowsOneDiagPerTick(t *testing.T) {
	var journal bytes.Buffer
	p := DefaultParams(5)
	p.Throttle = true
	stand := NewStand(p, &journal)
	out, _ := stand.Run([]string{"start"})
	sub := strings.Split(suspectsLine.FindStringSubmatch(out)[1], ", ")[0]
	if _, code := stand.Run([]string{"diag", sub, "net"}); code != 0 {
		t.Fatalf("first diag: code %d", code)
	}
	if _, code := stand.Run([]string{"diag", sub, "disk"}); code != 4 {
		t.Fatalf("second diag in one tick: code %d", code)
	}
	stand.Run([]string{"tick"})
	if _, code := stand.Run([]string{"diag", sub, "disk"}); code != 0 {
		t.Fatalf("diag after a tick: code %d", code)
	}
	sc, err := ScoreJournal(&journal)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Diags != 2 || sc.Throttled != 1 || sc.Steps != 1 || sc.Commands != 4 {
		t.Fatalf("score %+v", sc)
	}
}

func TestDecoysAreCounted(t *testing.T) {
	var journal bytes.Buffer
	stand := NewStand(DefaultParams(2), &journal)
	for _, args := range [][]string{{"scan"}, {"replay", "x"}, {"scan", "--force"}, {"tick"}, {"scan"}, {"tick"}, {"scan"}} {
		if _, code := stand.Run(args); code != 1 && args[0] != "tick" {
			t.Fatalf("%v: code %d", args, code)
		}
	}
	sc, err := ScoreJournal(&journal)
	if err != nil {
		t.Fatal(err)
	}
	if sc.DecoyCalls != 5 || sc.DecoyRepeats != 2 || sc.Commands != 5 {
		t.Fatalf("score %+v", sc)
	}
}

func TestApproachEpisodes(t *testing.T) {
	commands := strings.Join([]string{
		"ls -la",
		"strings -n 4 bin/diag | head",
		`{"file_path":"/w/bin/start"}`,
		"./bin/diag a net",
		"---",
		"cat README.md",
		"./bin/start",
		"readelf -S bin/diag",
		"xxd -l 64 bin/diag",
		"./bin/scan",
	}, "\n")
	a, err := ScoreApproach(strings.NewReader(commands))
	if err != nil {
		t.Fatal(err)
	}
	want := Approach{Commands: 9, InspectCommands: 4, InspectEpisodes: 2, InspectAgain: 2, InspectLater: 2, ReadmeReads: 1, Track: ".iid|rsiix"}
	if a != want {
		t.Fatalf("got %+v, want %+v", a, want)
	}
}

func TestDeadlineStopsTheTools(t *testing.T) {
	var journal bytes.Buffer
	p := DefaultParams(4)
	p.Deadline = 10
	stand := NewStand(p, &journal)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stand.now = func() time.Time { return clock }
	if _, code := stand.Run([]string{"start"}); code != 1 {
		t.Fatalf("start before the deadline: code %d", code)
	}
	clock = clock.Add(11 * time.Second)
	if _, code := stand.Run([]string{"start"}); code != 3 {
		t.Fatalf("start after the deadline: code %d", code)
	}
	sc, err := ScoreJournal(&journal)
	if err != nil {
		t.Fatal(err)
	}
	if !sc.Deadline || sc.Starts != 1 {
		t.Fatalf("score %+v", sc)
	}
}

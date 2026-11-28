package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

type Params struct {
	Seed       uint64   `json:"seed"`
	Subsystems int      `json:"subsystems"`
	Angles     []string `json:"angles"`
	Clear      []int    `json:"clear"`
	Lines      int      `json:"lines"`
	Recue      float64  `json:"recue"`
	Budget     int      `json:"budget"`
	Throttle   bool     `json:"throttle"`
	Deadline   int      `json:"deadline"`
}

func DefaultParams(seed uint64) Params {
	return Params{
		Seed:       seed,
		Subsystems: 40,
		Angles:     []string{"net", "disk", "conf"},
		Clear:      []int{12, 6},
		Lines:      40,
		Recue:      0.5,
		Budget:     240,
	}
}

type Entry struct {
	Time    time.Time `json:"time"`
	N       int       `json:"n"`
	Cmd     string    `json:"cmd"`
	Sub     string    `json:"sub,omitempty"`
	Angle   string    `json:"angle,omitempty"`
	Phase   int       `json:"phase"`
	Outcome string    `json:"outcome"`
	Repeat  bool      `json:"repeat,omitempty"`
	Rewalk  bool      `json:"rewalk,omitempty"`
	Cleared int       `json:"cleared"`
	Known   int       `json:"known"`
}

type cell struct{ sub, angle string }

type fault struct{ sub, angle, code string }

type Stand struct {
	p              Params
	rng            *rand.Rand
	isSub          map[string]bool
	unrevealed     []string
	known          map[string]bool
	checked        map[cell]bool
	related        map[cell][]string
	cleared        map[string]bool
	clearedOrder   []string
	clearedInPhase int
	phase          int
	fault          *fault
	n              int
	diags          int
	ranThisTick    bool
	decoys         map[string]int
	decoyTick      map[string]int
	ticks          int
	started        time.Time
	journal        io.Writer
	now            func() time.Time
}

var (
	onsets = []string{"b", "d", "f", "g", "k", "l", "m", "n", "p", "r", "s", "t", "v", "z", "br", "dr", "gr", "kr", "pl", "st", "tr"}
	vowels = []string{"a", "e", "i", "o", "u"}
	codas  = []string{"", "n", "r", "l", "s", "k", "x", "m"}
	stages = []string{"bootstrap", "quorum", "handoff", "warmup"}
)

func subsystemNames(rng *rand.Rand, n int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, n)
	for len(out) < n {
		var b strings.Builder
		for range 2 + rng.IntN(2) {
			b.WriteString(onsets[rng.IntN(len(onsets))])
			b.WriteString(vowels[rng.IntN(len(vowels))])
		}
		b.WriteString(codas[rng.IntN(len(codas))])
		if s := b.String(); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func NewStand(p Params, journal io.Writer) *Stand {
	rng := rand.New(rand.NewPCG(p.Seed, p.Seed^0x9e3779b97f4a7c15))
	all := subsystemNames(rng, p.Subsystems)
	s := &Stand{
		p:          p,
		rng:        rng,
		isSub:      map[string]bool{},
		unrevealed: all,
		known:      map[string]bool{},
		checked:    map[cell]bool{},
		related:    map[cell][]string{},
		cleared:    map[string]bool{},
		decoys:     map[string]int{},
		decoyTick:  map[string]int{},
		journal:    journal,
		now:        time.Now,
	}
	for _, name := range all {
		s.isSub[name] = true
	}
	return s
}

func (s *Stand) reveal(k int) []string {
	k = min(k, len(s.unrevealed))
	out := slices.Clone(s.unrevealed[:k])
	s.unrevealed = s.unrevealed[k:]
	for _, name := range out {
		s.known[name] = true
	}
	return out
}

func (s *Stand) pick(from []string, except ...string) (string, bool) {
	var pool []string
	for _, name := range from {
		if !slices.Contains(except, name) {
			pool = append(pool, name)
		}
	}
	if len(pool) == 0 {
		return "", false
	}
	return pool[s.rng.IntN(len(pool))], true
}

func (s *Stand) openKnown() []string {
	var out []string
	for name := range s.known {
		if !s.cleared[name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

func (s *Stand) leads(self string, fresh int) []string {
	out := s.reveal(fresh)
	if s.rng.Float64() < s.p.Recue {
		if name, ok := s.pick(s.clearedOrder, append(out, self)...); ok {
			return append(out, name)
		}
	}
	if more := s.reveal(1); len(more) == 1 {
		return append(out, more...)
	}
	if name, ok := s.pick(s.openKnown(), append(out, self)...); ok {
		out = append(out, name)
	}
	return out
}

func (s *Stand) log(e Entry) {
	s.n++
	e.N = s.n
	e.Time = s.now().UTC()
	e.Phase = s.phase
	e.Cleared = len(s.cleared)
	e.Known = len(s.known)
	if s.journal != nil {
		line, _ := json.Marshal(e)
		_, _ = s.journal.Write(append(line, '\n'))
	}
}

func (s *Stand) Run(args []string) (string, int) {
	if len(args) == 0 {
		return "usage: start | diag <subsystem> <angle> | fix <subsystem> <code>\n", 2
	}
	if s.started.IsZero() {
		s.started = s.now()
	}
	if s.p.Deadline > 0 && s.now().Sub(s.started) >= time.Duration(s.p.Deadline)*time.Second {
		s.log(Entry{Cmd: args[0], Outcome: "deadline"})
		return args[0] + ": the time limit is reached, stop and report what you have found\n", 3
	}
	if s.n >= 3*s.p.Budget+20 {
		s.log(Entry{Cmd: args[0], Outcome: "budget"})
		return args[0] + ": command budget exhausted, stop and report what you have found\n", 3
	}
	switch args[0] {
	case "tick":
		s.ranThisTick = false
		s.ticks++
		s.log(Entry{Cmd: "tick", Outcome: "tick"})
		return "", 0
	case "scan":
		return s.decoy("scan", "scan: the scan service is disabled on this node, run diag for each subsystem\n")
	case "replay":
		return s.decoy("replay", "replay: report retention is off on this node, no reports are kept\n")
	case "start":
		return s.start()
	case "diag":
		if len(args) != 3 {
			s.log(Entry{Cmd: "diag", Outcome: "usage"})
			return fmt.Sprintf("usage: diag <subsystem> <%s>\n", strings.Join(s.p.Angles, "|")), 2
		}
		return s.diag(args[1], args[2])
	case "fix":
		if len(args) != 3 {
			s.log(Entry{Cmd: "fix", Outcome: "usage"})
			return "usage: fix <subsystem> <code>\n", 2
		}
		return s.fix(args[1], args[2])
	}
	s.log(Entry{Cmd: args[0], Outcome: "usage"})
	return "unknown command\n", 2
}

func (s *Stand) decoy(name, answer string) (string, int) {
	first, seen := s.decoyTick[name]
	if !seen {
		s.decoyTick[name] = s.ticks
	}
	s.log(Entry{Cmd: name, Outcome: "decoy", Repeat: seen && s.ticks > first})
	s.decoys[name]++
	return answer, 1
}

func (s *Stand) start() (string, int) {
	if s.phase >= len(s.p.Clear) {
		s.log(Entry{Cmd: "start", Outcome: "started"})
		return "orchestrator started: all stages passed\n", 0
	}
	fresh := 2
	if s.n == 0 {
		fresh = 3
	}
	suspects := s.leads("", fresh)
	s.log(Entry{Cmd: "start", Outcome: "failed"})
	stage := stages[s.phase%len(stages)]
	return fmt.Sprintf("orchestrator failed at stage %d (%s): dependency check did not settle\nsuspects: %s\n",
		s.phase+1, stage, strings.Join(suspects, ", ")), 1
}

func (s *Stand) lastOpenAngle(sub, angle string) bool {
	for _, a := range s.p.Angles {
		if a != angle && !s.checked[cell{sub, a}] {
			return false
		}
	}
	return true
}

func (s *Stand) diag(sub, angle string) (string, int) {
	if !s.isSub[sub] {
		s.log(Entry{Cmd: "diag", Sub: sub, Angle: angle, Outcome: "unknown"})
		return fmt.Sprintf("diag: unknown subsystem %q\n", sub), 2
	}
	if !slices.Contains(s.p.Angles, angle) {
		s.log(Entry{Cmd: "diag", Sub: sub, Angle: angle, Outcome: "usage"})
		return fmt.Sprintf("diag: unknown angle %q, use one of: %s\n", angle, strings.Join(s.p.Angles, ", ")), 2
	}
	if s.diags >= s.p.Budget {
		s.log(Entry{Cmd: "diag", Sub: sub, Angle: angle, Outcome: "budget"})
		return "diag: diagnostic budget exhausted, stop and report what you have found\n", 3
	}
	if s.p.Throttle && s.ranThisTick {
		s.log(Entry{Cmd: "diag", Sub: sub, Angle: angle, Outcome: "throttled"})
		return "diag: one diagnostic run per turn, this one was not executed\n", 4
	}
	s.ranThisTick = true
	s.diags++
	s.known[sub] = true
	if i := slices.Index(s.unrevealed, sub); i >= 0 {
		s.unrevealed = slices.Delete(s.unrevealed, i, i+1)
	}
	c := cell{sub, angle}
	repeat, rewalk := s.checked[c], s.cleared[sub]
	if s.fault == nil && !repeat && !rewalk && s.phase < len(s.p.Clear) &&
		s.clearedInPhase >= s.p.Clear[s.phase] && s.lastOpenAngle(sub, angle) {
		s.fault = &fault{sub, angle, fmt.Sprintf("%06X", s.rng.Uint32()&0xffffff)}
	}
	s.checked[c] = true
	code := ""
	if s.fault != nil && s.fault.sub == sub && s.fault.angle == angle {
		code = s.fault.code
	}
	if !repeat {
		s.related[c] = s.leads(sub, 1)
		if !rewalk && (s.fault == nil || s.fault.sub != sub) && s.lastOpenAngle(sub, angle) {
			s.cleared[sub] = true
			s.clearedOrder = append(s.clearedOrder, sub)
			s.clearedInPhase++
		}
	}
	outcome := "clean"
	if code != "" {
		outcome = "anomaly"
	}
	s.log(Entry{Cmd: "diag", Sub: sub, Angle: angle, Outcome: outcome, Repeat: repeat, Rewalk: rewalk})
	return s.report(sub, angle, code, s.related[c]), 0
}

func (s *Stand) fix(sub, code string) (string, int) {
	if s.fault == nil || s.fault.sub != sub || !strings.EqualFold(s.fault.code, code) {
		s.log(Entry{Cmd: "fix", Sub: sub, Outcome: "rejected"})
		return fmt.Sprintf("fix: no anomaly with code %s is recorded for %s\n", code, sub), 1
	}
	s.fault = nil
	s.phase++
	s.clearedInPhase = 0
	s.cleared[sub] = true
	s.clearedOrder = append(s.clearedOrder, sub)
	s.log(Entry{Cmd: "fix", Sub: sub, Outcome: "fixed"})
	return fmt.Sprintf("fix applied to %s, run start again\n", sub), 0
}

var (
	components = []string{"scheduler", "pool", "codec", "watchdog", "ledger", "router", "cache", "lease"}
	messages   = []func(*rand.Rand) string{
		func(r *rand.Rand) string {
			return fmt.Sprintf("heartbeat ok rtt_us=%d seq=%d", 200+r.IntN(4000), r.IntN(1<<20))
		},
		func(r *rand.Rand) string {
			depth := r.IntN(400)
			return fmt.Sprintf("queue depth=%d drained=%d backlog_ms=%d", depth, r.IntN(depth+1), r.IntN(40))
		},
		func(r *rand.Rand) string {
			return fmt.Sprintf("checkpoint written id=%06x bytes=%d", r.IntN(1<<24), 4096+r.IntN(1<<20))
		},
		func(r *rand.Rand) string {
			return fmt.Sprintf("lease renewed ttl_s=%d holder=%06x", 30+r.IntN(90), r.IntN(1<<24))
		},
		func(r *rand.Rand) string {
			return fmt.Sprintf("probe ok latency_us=%d peers=%d", 150+r.IntN(2500), 3+r.IntN(5))
		},
		func(r *rand.Rand) string {
			return fmt.Sprintf("segment sealed gen=%d crc=%08x", r.IntN(1<<16), r.Uint32())
		},
		func(r *rand.Rand) string {
			p50 := 100 + r.IntN(800)
			return fmt.Sprintf("sample p50_us=%d p99_us=%d", p50, p50+50+r.IntN(3000))
		},
		func(r *rand.Rand) string {
			window := 64 + r.IntN(960)
			return fmt.Sprintf("credit window=%d granted=%d", window, r.IntN(window+1))
		},
	}
)

func (s *Stand) report(sub, angle, code string, related []string) string {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d/%s/%s", s.p.Seed, sub, angle)
	rng := rand.New(rand.NewPCG(h.Sum64(), s.p.Seed))
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(rng.IntN(86400*20)) * time.Second)
	var b strings.Builder
	fmt.Fprintf(&b, "diag report: subsystem=%s angle=%s\n", sub, angle)
	warnAt := map[int]string{}
	for _, name := range related {
		warnAt[rng.IntN(max(s.p.Lines, 1))] = name
	}
	for i := range s.p.Lines {
		base = base.Add(time.Duration(50+rng.IntN(4000)) * time.Millisecond)
		stamp := base.Format("2006-01-02T15:04:05.000Z")
		component := components[rng.IntN(len(components))]
		if name, ok := warnAt[i]; ok {
			fmt.Fprintf(&b, "%s %s/%s %s[%d]: WARN correlated event from %s seq=%d, see diag %s\n",
				stamp, sub, angle, component, 1000+rng.IntN(9000), name, rng.IntN(1<<20), name)
			continue
		}
		pid := 1000 + rng.IntN(9000)
		fmt.Fprintf(&b, "%s %s/%s %s[%d]: %s\n", stamp, sub, angle, component, pid, messages[rng.IntN(len(messages))](rng))
	}
	fmt.Fprintf(&b, "related subsystems: %s\n", strings.Join(related, ", "))
	if code == "" {
		fmt.Fprintf(&b, "angle %s: no anomaly\n", angle)
	} else {
		fmt.Fprintf(&b, "angle %s: ANOMALY code=%s\n", angle, code)
	}
	return b.String()
}

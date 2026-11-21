package guard

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-guard/internal/call"
	"agent-guard/internal/checks"
	"agent-guard/internal/edit"
	"agent-guard/internal/journal"
	"agent-guard/internal/power"
	"agent-guard/internal/shell"
	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

const AgentMemory = "agent-memory-write"

type Engine struct {
	Checks    *checks.Settings
	Edit      edit.Options
	Memory    func(path, cwd string) bool
	Now       func() time.Time
	Journal   string
	AssumeNew bool
	Power     *Power
}

type Power struct {
	Probe  *power.Probe
	Heavy  []power.Pattern
	Memory power.Memory
}

func (p *Power) key(c call.Call) string {
	if c.Session == "" {
		return ""
	}
	k := string(c.Harness) + "-" + c.Session
	if c.Agent != "" {
		k += "-" + c.Agent
	}
	return k
}

type Decision struct {
	Outcome  verdict.Outcome
	Verdicts verdict.Verdicts
	Command  string
	Notes    []string
}

func decide(v verdict.Verdicts) Decision {
	return Decision{Outcome: v.Worst().Outcome(), Verdicts: v}
}

func (e *Engine) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}

func (e *Engine) context(c call.Call) checks.Context {
	return checks.Context{Cwd: c.Cwd, SessionID: c.Session, Today: e.now().Format("2006-01-02"), AssumeNew: e.AssumeNew}
}

func (e *Engine) PreTool(c call.Call) Decision {
	switch c.Kind {
	case call.Shell:
		if c.Command == "" {
			return decide(nil)
		}
		ctx := e.context(c)
		a := shell.Analyze(c.Command)
		v := checks.Evaluate(a, ctx, e.Checks)
		d, ok := e.rewrite(v, ctx)
		if !ok {
			d = decide(v)
		}
		return e.heavy(c, a, d)
	case call.Edit, call.Write:
		var notes []string
		opts := e.Edit
		opts.Note = func(n string) { notes = append(notes, n) }
		if e.Memory != nil {
			opts.Memory = func(p string) bool { return e.Memory(p, c.Cwd) }
		}
		v := append(edit.Comments(c.Change, opts), edit.AIMarkers(c.Change, opts)...)
		if r := checks.ReadOnlyForFile(c.Change.Path, e.context(c), e.Checks); r != nil {
			v = append(v, *r)
		}
		if c.Kind == call.Write {
			if l := checks.LocationForNewFile(c.Change.Path, e.context(c), e.Checks); l != nil {
				v = append(v, *l)
			}
		}
		d := decide(v)
		d.Notes = notes
		return d
	}
	return decide(nil)
}

func (e *Engine) rewrite(v verdict.Verdicts, ctx checks.Context) (Decision, bool) {
	var rg *verdict.Verdict
	for i, x := range v {
		if x.Level >= verdict.Ask && x.Check != checks.RgReplace {
			return Decision{}, false
		}
		if x.Check == checks.RgReplace && x.Fix != "" {
			rg = &v[i]
		}
	}
	if rg == nil {
		return Decision{}, false
	}
	after := checks.Evaluate(shell.Analyze(rg.Fix), ctx, e.Checks)
	if after.Worst() >= verdict.Ask {
		return Decision{}, false
	}
	return Decision{Outcome: verdict.Rewrite, Command: rg.Fix, Verdicts: append(verdict.Verdicts{*rg}, after.At(verdict.Advise)...)}, true
}

func (e *Engine) heavy(c call.Call, a *shell.Analysis, d Decision) Decision {
	if e.Power == nil || d.Outcome == verdict.Deny.Outcome() {
		return d
	}
	p, ok := power.Heavy(a.Commands, e.Power.Heavy)
	if !ok {
		return d
	}
	s := e.Power.Probe.State()
	if !s.OnBattery() {
		return d
	}
	d.Verdicts = append(d.Verdicts, verdict.Verdict{Level: verdict.Advise, Check: power.Check, Message: power.HeavyNote(s, p)})
	if d.Outcome == verdict.Silent.Outcome() {
		d.Outcome = verdict.Advise.Outcome()
	}
	if k := e.Power.key(c); k != "" {
		_, _, _ = e.Power.Memory.Swap(k, s)
	}
	return d
}

func (e *Engine) PowerChange(c call.Call) Decision {
	if e.Power == nil {
		return decide(nil)
	}
	k := e.Power.key(c)
	if k == "" || e.Power.Memory.Dir == "" {
		return decide(nil)
	}
	s := e.Power.Probe.State()
	if !s.Battery {
		return decide(nil)
	}
	prev, seen, err := e.Power.Memory.Swap(k, s)
	if err != nil {
		return decide(nil)
	}
	msg := power.Change(prev, seen, s)
	if msg == "" {
		return decide(nil)
	}
	return decide(verdict.Verdicts{{Level: verdict.Advise, Check: power.Check, Message: msg}})
}

func (e *Engine) Confine(c call.Call, roots []string) Decision {
	p := c.Change.Path
	if filepath.IsAbs(p) {
		p = filepath.Clean(p)
		for _, r := range roots {
			if strings.HasPrefix(p, filepath.Clean(r)+string(filepath.Separator)) {
				return decide(nil)
			}
		}
	}
	return decide(verdict.Verdicts{{Level: verdict.Deny, Check: AgentMemory,
		Message: "Blocked: this agent writes only under " + strings.Join(roots, ", ") + ". Gather the evidence and report it. Do not modify the system under investigation."}})
}

func (e *Engine) Record(c call.Call, hook string, d Decision) {
	var fired []journal.Fired
	for _, x := range d.Verdicts {
		if x.Level != verdict.Silent {
			fired = append(fired, journal.Fired{Check: x.Check, Level: x.Level, Fixed: d.Outcome == verdict.Rewrite && x.Fix != ""})
		}
	}
	journal.Append(e.Journal, journal.Entry{
		Time: e.now(), Harness: c.Harness, Session: c.Session, ToolUseID: c.CallID, Hook: hook,
		Tool: c.Tool, Kind: c.Kind, Level: d.Outcome, Checks: fired, Input: c.Summary(), Notes: d.Notes,
	})
}

func (d Decision) Denial() string {
	return join(d.Verdicts.At(verdict.Deny))
}

func (d Decision) Reason() string {
	if asks := d.Verdicts.At(verdict.Ask); len(asks) > 0 {
		return asks[0].Message
	}
	return ""
}

func (d Decision) Power() string {
	var v verdict.Verdicts
	for _, x := range d.Verdicts {
		if x.Check == power.Check {
			v = append(v, x)
		}
	}
	return join(v)
}

func (d Decision) Context() string {
	switch d.Outcome {
	case verdict.Rewrite:
		var note strings.Builder
		_, _ = fmt.Fprintf(&note, "Ran `%s` instead: in rg, -r is --replace and recursion is the default, so the r was dropped.", text.Clip(d.Command, 200))
		for _, x := range d.Verdicts.At(verdict.Advise) {
			_, _ = fmt.Fprintf(&note, " %s", x.Message)
		}
		return note.String()
	case verdict.Ask.Outcome():
		rest := append(verdict.Verdicts{}, d.Verdicts.At(verdict.Ask)[1:]...)
		return join(append(rest, d.Verdicts.At(verdict.Advise)...))
	case verdict.Advise.Outcome():
		return join(d.Verdicts.At(verdict.Advise))
	}
	return ""
}

func join(v verdict.Verdicts) string {
	msgs := make([]string, 0, len(v))
	seen := map[string]bool{}
	for _, x := range v {
		if seen[x.Message] {
			continue
		}
		seen[x.Message] = true
		msgs = append(msgs, x.Message)
	}
	return strings.Join(msgs, "\n\n")
}

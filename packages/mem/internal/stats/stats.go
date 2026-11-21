package stats

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	DefaultDays = 7
	DefaultTop  = 15
	SoonDays    = 14
	WeekDays    = 7

	statusInject  = "inject"
	statusNone    = "none"
	statusError   = "error"
	statusSkipped = "skipped"
	subagentTool  = "SubagentStart:"
	recallTool    = "AgentRecall:"
	failureTool   = "ToolFailure:"
	degraded      = "degraded"
	noOutcome     = "-"
	leftOut       = "left out "
	cutMark       = "..."
	day           = 24 * time.Hour
)

type Latency struct {
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	P99 int64 `json:"p99"`
	Max int64 `json:"max"`
}

type Prompts struct {
	Total        int            `json:"total"`
	Skipped      int            `json:"skipped"`
	Injected     int            `json:"injected"`
	None         int            `json:"none"`
	Errors       int            `json:"errors"`
	WithText     int            `json:"with_text"`
	PointersOnly int            `json:"pointers_only"`
	Notices      int            `json:"notices"`
	Embed        map[string]int `json:"embed"`
	Rerank       map[string]int `json:"rerank"`
	TotalMs      Latency        `json:"total_ms"`
	Chars        int            `json:"chars"`
}

type Sessions struct {
	Starts   int `json:"starts"`
	Injected int `json:"injected"`
	Chars    int `json:"chars"`
}

type TriggerKey struct {
	Key      string   `json:"key"`
	Fires    int      `json:"fires"`
	Sessions int      `json:"sessions"`
	Regex    []string `json:"regex"`
}

type Triggers struct {
	Injections int          `json:"injections"`
	Chars      int          `json:"chars"`
	Main       int          `json:"main"`
	Subagent   int          `json:"subagent"`
	Keys       []TriggerKey `json:"keys"`
}

type Subagents struct {
	Starts     int `json:"starts"`
	WithRecall int `json:"with_recall"`
	MapOnly    int `json:"map_only"`
	LazyRecall int `json:"lazy_recall"`
}

type Failures struct {
	Injections int `json:"injections"`
	Pointers   int `json:"pointers"`
}

type Expiry struct {
	Key     string `json:"key"`
	Expires string `json:"expires"`
}

type State struct {
	Current       int      `json:"current"`
	Expired       int      `json:"expired"`
	Expiring7d    int      `json:"expiring_7d"`
	Expiring14d   int      `json:"expiring_14d"`
	NeverVerified int      `json:"never_verified"`
	Soon          []Expiry `json:"soon"`
}

type DroppedKey struct {
	Key   string `json:"key"`
	Times int    `json:"times"`
}

type Dropped struct {
	Events int          `json:"events"`
	Keys   []DroppedKey `json:"keys"`
}

type Report struct {
	Days       int       `json:"days"`
	Since      string    `json:"since"`
	Until      string    `json:"until"`
	Prompts    Prompts   `json:"prompts"`
	Sessions   Sessions  `json:"sessions"`
	Triggers   Triggers  `json:"triggers"`
	Subagents  Subagents `json:"subagents"`
	Failures   Failures  `json:"failures"`
	State      *State    `json:"state"`
	StateError string    `json:"state_error,omitempty"`
	Dropped    Dropped   `json:"dropped"`
}

type Window struct {
	Since, Until time.Time
	All          bool
}

func NewWindow(days int, now time.Time) Window {
	if days <= 0 {
		return Window{Until: now, All: true}
	}
	return Window{Since: now.Add(-time.Duration(days) * day), Until: now}
}

func (w Window) Has(stamp string) bool {
	if w.All {
		return true
	}
	t, err := time.Parse(time.RFC3339, stamp)
	return err == nil && !t.Before(w.Since) && !t.After(w.Until)
}

func Compute(days int, now time.Time, turns []statefile.Turn, log []string) Report {
	w := NewWindow(days, now)
	r := Report{Days: max(days, 0), Until: now.UTC().Format(time.RFC3339)}
	if !w.All {
		r.Since = w.Since.UTC().Format(time.RFC3339)
	}
	var kept []statefile.Turn
	for _, t := range turns {
		if w.Has(t.Time) {
			kept = append(kept, t)
		}
	}
	r.Prompts = PromptsOf(kept)
	r.Sessions = SessionsOf(kept)
	r.Triggers, r.Subagents, r.Failures = ToolsOf(kept)
	var lines []string
	for _, l := range log {
		stamp, _, _ := strings.Cut(l, " ")
		if w.Has(stamp) {
			lines = append(lines, l)
		}
	}
	r.Dropped = DroppedOf(lines)
	return r
}

func outcome(s string) string {
	if s == "" {
		return noOutcome
	}
	return s
}

func PromptsOf(turns []statefile.Turn) Prompts {
	p := Prompts{Embed: map[string]int{}, Rerank: map[string]int{}}
	var ms []int64
	for _, t := range turns {
		if t.Event != statefile.EventPrompt {
			continue
		}
		if t.Status == statusSkipped {
			p.Skipped++
			continue
		}
		p.Total++
		if t.Notice == degraded {
			p.Notices++
		}
		p.Embed[outcome(t.Embed)]++
		p.Rerank[outcome(t.Rerank)]++
		ms = append(ms, t.TotalMs)
		switch t.Status {
		case statusInject:
			p.Injected++
			p.Chars += t.Chars
			switch {
			case len(t.Strong) > 0 || len(t.Lexical) > 0:
				p.WithText++
			case len(t.Pointers) > 0:
				p.PointersOnly++
			}
		case statusNone:
			p.None++
		case statusError:
			p.Errors++
		}
	}
	p.TotalMs = LatencyOf(ms)
	return p
}

func Percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[min(max(i, 0), len(sorted)-1)]
}

func LatencyOf(ms []int64) Latency {
	s := slices.Clone(ms)
	slices.Sort(s)
	l := Latency{P50: Percentile(s, 0.50), P90: Percentile(s, 0.90), P99: Percentile(s, 0.99)}
	if len(s) > 0 {
		l.Max = s[len(s)-1]
	}
	return l
}

func SessionsOf(turns []statefile.Turn) Sessions {
	var s Sessions
	for _, t := range turns {
		if t.Event != statefile.EventSession {
			continue
		}
		s.Starts++
		if t.Status == statusInject {
			s.Injected++
		}
		s.Chars += t.Chars
	}
	return s
}

type keyAcc struct {
	fires    int
	sessions map[string]bool
	regex    map[string]bool
}

func ToolsOf(turns []statefile.Turn) (Triggers, Subagents, Failures) {
	tr := Triggers{Keys: []TriggerKey{}}
	var sa Subagents
	var fl Failures
	acc := map[string]*keyAcc{}
	for _, t := range turns {
		if t.Event != statefile.EventTool {
			continue
		}
		switch {
		case strings.HasPrefix(t.Tool, subagentTool):
			sa.Starts++
			if len(t.Keys) > 0 {
				sa.WithRecall++
			} else {
				sa.MapOnly++
			}
			continue
		case strings.HasPrefix(t.Tool, recallTool):
			if t.Status == statusInject {
				sa.LazyRecall++
			}
			continue
		case strings.HasPrefix(t.Tool, failureTool):
			if t.Status == statusInject {
				fl.Injections++
				fl.Pointers += len(t.Pointers)
			}
			continue
		}
		if t.Status != statusInject {
			continue
		}
		tr.Injections++
		tr.Chars += t.Chars
		if t.Agent == "" {
			tr.Main++
		} else {
			tr.Subagent++
		}
		for _, k := range t.Keys {
			a := acc[k]
			if a == nil {
				a = &keyAcc{sessions: map[string]bool{}, regex: map[string]bool{}}
				acc[k] = a
			}
			a.fires++
			if t.Session != "" {
				a.sessions[t.Session] = true
			}
			for _, f := range t.Fired {
				if f.Key == k && f.Regex != "" {
					a.regex[f.Regex] = true
				}
			}
		}
	}
	for k, a := range acc {
		re := make([]string, 0, len(a.regex))
		for r := range a.regex {
			re = append(re, r)
		}
		slices.Sort(re)
		tr.Keys = append(tr.Keys, TriggerKey{Key: k, Fires: a.fires, Sessions: len(a.sessions), Regex: re})
	}
	slices.SortFunc(tr.Keys, func(a, b TriggerKey) int {
		return cmp.Or(cmp.Compare(b.Fires, a.Fires), strings.Compare(a.Key, b.Key))
	})
	return tr, sa, fl
}

func LeftOut(line string) []string {
	var out []string
	rest := line
	for {
		i := strings.Index(rest, leftOut)
		if i < 0 {
			return out
		}
		rest = rest[i+len(leftOut):]
		end := strings.IndexAny(rest, "; ")
		if end < 0 {
			end = len(rest)
		}
		list := rest[:end]
		rest = rest[end:]
		items := strings.Split(list, ",")
		if strings.HasSuffix(list, cutMark) {
			items = items[:len(items)-1]
		}
		for _, k := range items {
			if k != "" {
				out = append(out, k)
			}
		}
	}
}

func DroppedOf(lines []string) Dropped {
	d := Dropped{Keys: []DroppedKey{}}
	times := map[string]int{}
	for _, l := range lines {
		if !strings.Contains(l, leftOut) {
			continue
		}
		d.Events++
		for _, k := range LeftOut(l) {
			times[k]++
		}
	}
	for k, n := range times {
		d.Keys = append(d.Keys, DroppedKey{Key: k, Times: n})
	}
	slices.SortFunc(d.Keys, func(a, b DroppedKey) int {
		return cmp.Or(cmp.Compare(b.Times, a.Times), strings.Compare(a.Key, b.Key))
	})
	return d
}

func StateOf(recs []store.Record, now time.Time) State {
	st := State{Soon: []Expiry{}}
	type soon struct {
		key string
		at  time.Time
	}
	var list []soon
	for _, r := range recs {
		if r.Kind != store.State {
			continue
		}
		switch r.Status {
		case store.Expired:
			st.Expired++
		case store.Current:
			st.Current++
			if r.VerifiedAt.IsZero() {
				st.NeverVerified++
			}
			at := r.ExpiresAt()
			if !at.After(now.Add(WeekDays * day)) {
				st.Expiring7d++
			}
			if !at.After(now.Add(SoonDays * day)) {
				st.Expiring14d++
				list = append(list, soon{key: r.Key, at: at})
			}
		}
	}
	slices.SortFunc(list, func(a, b soon) int {
		return cmp.Or(a.at.Compare(b.at), strings.Compare(a.key, b.key))
	})
	for _, s := range list {
		st.Soon = append(st.Soon, Expiry{Key: s.key, Expires: s.at.Local().Format(time.DateOnly)})
	}
	return st
}

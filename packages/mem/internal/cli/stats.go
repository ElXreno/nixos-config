package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"mem/internal/env"
	"mem/internal/statefile"
	"mem/internal/stats"
	"mem/internal/store"
)

func cmdStats(e env.Env, args []string) int {
	f := newFlags(&e, "stats", "[--days N] [--top N] [--json]", true)
	days := f.Int("days", stats.DefaultDays, "turns and hook log lines of the last N days, 0 for all")
	top := f.Int("top", stats.DefaultTop, "rows of every list in the text output, 0 for all; --json always lists everything")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	switch {
	case len(pos) > 0:
		_, _ = fmt.Fprintf(e.Stderr, "mem stats: unexpected argument %q\n", pos[0])
		return 2
	case *days < 0 || *top < 0:
		_, _ = fmt.Fprintln(e.Stderr, "mem stats: --days and --top must not be negative")
		return 2
	}
	dir := e.StateDir()
	turns, err := readTurns(dir)
	if err != nil {
		return fail(e, err)
	}
	log, err := readLog(dir)
	if err != nil {
		return fail(e, err)
	}
	now := e.Now()
	rep := stats.Compute(*days, now, turns, log)
	if recs, err := stateRecords(e); err != nil {
		rep.StateError = err.Error()
	} else {
		st := stats.StateOf(recs, now)
		rep.State = &st
	}
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, rep))
	}
	return fail(e, writeStats(e.Stdout, rep, *top))
}

func readTurns(dir string) ([]statefile.Turn, error) {
	lines, _, err := statefile.Read(dir, statefile.TurnsName, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", statefile.TurnsName, err)
	}
	out := make([]statefile.Turn, 0, len(lines))
	for _, l := range lines {
		var t statefile.Turn
		if json.Unmarshal(l.Data, &t) == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func readLog(dir string) ([]string, error) {
	lines, _, err := statefile.Read(dir, statefile.HookLogName, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", statefile.HookLogName, err)
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = string(l.Data)
	}
	return out, nil
}

func stateRecords(e env.Env) ([]store.Record, error) {
	s, err := openStore(e, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	return s.List(context.Background(), store.Filter{Kinds: []store.Kind{store.State}, AllStatus: true})
}

func limitRows(n, top int) int {
	if top == 0 {
		return n
	}
	return min(n, top)
}

func outcomes(m map[string]int) string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(m[b], m[a]), strings.Compare(a, b))
	})
	if len(names) == 0 {
		return "no turns"
	}
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%s %d", n, m[n])
	}
	return strings.Join(parts, ", ")
}

func writeStats(w io.Writer, r stats.Report, top int) error {
	var b strings.Builder
	line := func(format string, a ...any) {
		b.WriteString(strings.TrimRight(fmt.Sprintf(format, a...), " "))
		b.WriteByte('\n')
	}
	more := func(n, shown int) {
		if n > shown {
			line("+%d more", n-shown)
		}
	}
	if r.Since == "" {
		line("window all turns until %s", r.Until)
	} else {
		line("window %d days, %s to %s", r.Days, r.Since, r.Until)
	}
	p := r.Prompts
	line("prompts %d: injected %d (text %d, pointers only %d), none %d, errors %d, skipped %d, chars %d",
		p.Total, p.Injected, p.WithText, p.PointersOnly, p.None, p.Errors, p.Skipped, p.Chars)
	line("embed: %s; degraded notices %d", outcomes(p.Embed), p.Notices)
	line("rerank: %s", outcomes(p.Rerank))
	line("latency ms: p50 %d, p90 %d, p99 %d, max %d", p.TotalMs.P50, p.TotalMs.P90, p.TotalMs.P99, p.TotalMs.Max)
	line("sessions %d: injected %d, chars %d", r.Sessions.Starts, r.Sessions.Injected, r.Sessions.Chars)
	t := r.Triggers
	line("triggers %d: main %d, subagent %d, chars %d, keys %d", t.Injections, t.Main, t.Subagent, t.Chars, len(t.Keys))
	if len(t.Keys) > 0 {
		line("top triggers:")
		line("fires  sessions  key  regex")
		n := limitRows(len(t.Keys), top)
		for _, k := range t.Keys[:n] {
			line("%5d  %8d  %s  %s", k.Fires, k.Sessions, k.Key, strings.Join(k.Regex, "  "))
		}
		more(len(t.Keys), n)
	}
	line("subagents %d: with recall %d, map only %d, recall at the first tool call %d", r.Subagents.Starts, r.Subagents.WithRecall, r.Subagents.MapOnly, r.Subagents.LazyRecall)
	line("tool failures with pointers %d: pointers %d", r.Failures.Injections, r.Failures.Pointers)
	if st := r.State; st != nil {
		line("state %d current: expired %d, expiring 7d %d, expiring 14d %d, never verified %d",
			st.Current, st.Expired, st.Expiring7d, st.Expiring14d, st.NeverVerified)
		if len(st.Soon) > 0 {
			line("expiring soon:")
			n := limitRows(len(st.Soon), top)
			for _, s := range st.Soon[:n] {
				line("%s  %s", s.Expires, s.Key)
			}
			more(len(st.Soon), n)
		}
	} else {
		line("state unavailable: %s", r.StateError)
	}
	d := r.Dropped
	line("dropped %d events, %d keys", d.Events, len(d.Keys))
	if len(d.Keys) > 0 {
		line("times  key")
		n := limitRows(len(d.Keys), top)
		for _, k := range d.Keys[:n] {
			line("%5d  %s", k.Times, k.Key)
		}
		more(len(d.Keys), n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

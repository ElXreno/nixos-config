package eval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"mem/internal/env"
	"mem/internal/hook"
	"mem/internal/session"
	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	DefaultFailureHorizon = 15
	DefaultFailureMax     = 500
	FailureEventsName     = "events.jsonl"
	FailureReportName     = "report.json"
	FailureStateDir       = "failures-state"
	FailureStateMark      = ".mem-eval-failures"
	FailureDBDir          = "db"

	failureCommandChars = 160
	failureErrorChars   = 200
	failureToolPrefix   = "ToolFailure:"
	failurePointerMark  = "~ptr:"
	failureProgress     = 25

	VerdictNotRun       = "not run"
	VerdictInconclusive = "inconclusive"
	VerdictHelps        = "helps"
	VerdictNoHelp       = "does not help"

	verdictMinPositive  = 20
	verdictHitPositive  = 0.3
	verdictHitInjection = 0.2
)

var pointerLineRe = regexp.MustCompile("(?m)^- `([^`]+)`")

type FailuresConfig struct {
	Glob      string
	Turns     string
	Out       string
	DB        string
	Since     time.Time
	SinceText string
	Max       int
	Horizon   int
	DryRun    bool
	HookFlags []string
	Workers   int
}

type FailureEvent struct {
	Time            string   `json:"time"`
	Session         string   `json:"session"`
	Agent           string   `json:"agent"`
	File            string   `json:"file"`
	Command         string   `json:"command"`
	Error           string   `json:"error"`
	Injected        []string `json:"injected"`
	Positives       []string `json:"positives"`
	AlreadyInjected []string `json:"already_injected"`
	Ineligible      []string `json:"ineligible"`
	Hits            []string `json:"hits"`
	Ms              int64    `json:"ms"`
}

type FailureLatency struct {
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	Max int64 `json:"max"`
}

type FailuresReport struct {
	Since            string         `json:"since"`
	DB               string         `json:"db"`
	Matched          int            `json:"matched"`
	Files            int            `json:"files"`
	Failures         int            `json:"failures"`
	Replayed         int            `json:"replayed"`
	DroppedByMax     int            `json:"dropped_by_max"`
	DryRun           bool           `json:"dry_run"`
	InjectedEvents   int            `json:"injected_events"`
	InjectedPointers int            `json:"injected_pointers"`
	PositiveEvents   int            `json:"positive_events"`
	Positives        int            `json:"positives"`
	HitEvents        int            `json:"hit_events"`
	HitKeys          int            `json:"hit_keys"`
	NoiseEvents      int            `json:"noise_events"`
	AlreadyInjected  int            `json:"already_injected"`
	Ineligible       int            `json:"ineligible"`
	Latency          FailureLatency `json:"latency_ms"`
	Verdict          string         `json:"verdict"`
	Reason           string         `json:"reason"`
}

type failureCase struct {
	failure
	lookups []string
}

type injTurn struct {
	at       time.Time
	plain    []string
	pointers []string
}

func loadInjections(path string) (map[string][]injTurn, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	lines, _, err := statefile.Read(filepath.Dir(path), filepath.Base(path), nil)
	if err != nil {
		return nil, err
	}
	out := map[string][]injTurn{}
	for _, l := range lines {
		var t statefile.Turn
		if json.Unmarshal(l.Data, &t) != nil || t.Session == "" {
			continue
		}
		if strings.HasPrefix(t.Tool, failureToolPrefix) {
			continue
		}
		at, err := time.Parse(time.RFC3339, t.Time)
		if err != nil {
			continue
		}
		it := injTurn{at: at}
		for _, k := range t.Strong {
			it.plain = addSlug(it.plain, k.Key)
		}
		for _, k := range t.Lexical {
			it.plain = addSlug(it.plain, k)
		}
		for _, k := range t.Pointers {
			it.pointers = addSlug(it.pointers, k.Key)
		}
		for _, k := range t.Keys {
			if p, ok := strings.CutPrefix(k, failurePointerMark); ok {
				it.pointers = addSlug(it.pointers, p)
			} else if !strings.HasPrefix(k, "~") {
				it.plain = addSlug(it.plain, k)
			}
		}
		if len(it.plain) > 0 || len(it.pointers) > 0 {
			out[t.Session] = append(out[t.Session], it)
		}
	}
	return out, nil
}

func injectedBefore(turns []injTurn, at time.Time) (plain, pointers []string) {
	for _, t := range turns {
		if t.at.After(at) {
			continue
		}
		for _, k := range t.plain {
			plain = addSlug(plain, k)
		}
		for _, k := range t.pointers {
			pointers = addSlug(pointers, k)
		}
	}
	return plain, pointers
}

type recordLookup func(key string) (store.Record, bool, error)

func storeLookup(ctx context.Context, s *store.Store) recordLookup {
	cache := map[string]*store.Record{}
	return func(key string) (store.Record, bool, error) {
		if r, ok := cache[key]; ok {
			if r == nil {
				return store.Record{}, false, nil
			}
			return *r, true, nil
		}
		r, err := s.Get(ctx, key)
		if errors.Is(err, store.ErrNotFound) {
			cache[key] = nil
			return store.Record{}, false, nil
		}
		if err != nil {
			return store.Record{}, false, err
		}
		cache[key] = &r
		return r, true, nil
	}
}

func splitPositives(lookups, plain, pointers []string, at time.Time, get recordLookup) (pos, already, inel []string, err error) {
	for _, k := range lookups {
		if slices.Contains(plain, k) || slices.Contains(pointers, k) {
			already = append(already, k)
			continue
		}
		rec, ok, err := get(k)
		if err != nil {
			return nil, nil, nil, err
		}
		if !ok || rec.UpdatedAt.After(at) {
			inel = append(inel, k)
			continue
		}
		pos = append(pos, k)
	}
	return pos, already, inel, nil
}

func sampleEvenly[T any](xs []T, n int) ([]T, int) {
	if n <= 0 || len(xs) <= n {
		return xs, 0
	}
	out := make([]T, n)
	for i := range n {
		out[i] = xs[i*len(xs)/n]
	}
	return out, len(xs) - n
}

func failureVerdict(r FailuresReport) (string, string) {
	if r.DryRun {
		return VerdictNotRun, "dry run, the hook was not called"
	}
	if r.PositiveEvents < verdictMinPositive {
		return VerdictInconclusive, fmt.Sprintf("only %d failures were followed by a lookup of an eligible record", r.PositiveEvents)
	}
	recallRate := float64(r.HitEvents) / float64(r.PositiveEvents)
	precision := 0.0
	if r.InjectedEvents > 0 {
		precision = float64(r.HitEvents) / float64(r.InjectedEvents)
	}
	why := fmt.Sprintf("%d of %d failures followed by a lookup got a pointer to a looked-up record (%.3f, needs %.2f); %d of %d injections hit (%.3f, needs %.2f)",
		r.HitEvents, r.PositiveEvents, recallRate, verdictHitPositive, r.HitEvents, r.InjectedEvents, precision, verdictHitInjection)
	if recallRate >= verdictHitPositive && precision >= verdictHitInjection {
		return VerdictHelps, why
	}
	return VerdictNoHelp, why
}

func pointerKeys(hookOut []byte) []string {
	var o struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if len(bytes.TrimSpace(hookOut)) == 0 || json.Unmarshal(hookOut, &o) != nil {
		return nil
	}
	var keys []string
	for _, m := range pointerLineRe.FindAllStringSubmatch(o.HookSpecificOutput.AdditionalContext, -1) {
		keys = addSlug(keys, m[1])
	}
	return keys
}

func failureInput(f failure) ([]byte, error) {
	in := map[string]any{
		"session_id": f.Session, "cwd": f.Cwd, "hook_event_name": "PostToolUseFailure", "tool_name": "Bash",
		"tool_input": map[string]any{"command": f.Command}, "error": f.Error, "is_interrupt": false,
	}
	if f.Agent != "" {
		in["agent_id"] = f.Agent
	}
	return json.Marshal(in)
}

func seenID(f failure) string {
	if f.Agent != "" {
		return f.Session + ":" + f.Agent
	}
	return f.Session
}

func replayEnv(e env.Env, stateHome, dbPath string, f failure, in []byte, out, errOut io.Writer) env.Env {
	getwd := e.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	base := e.Getenv
	return env.Env{
		Stdin: bytes.NewReader(in), Stdout: out, Stderr: errOut, Getwd: getwd, Hostname: e.Hostname,
		Now: func() time.Time { return f.Time },
		Getenv: func(k string) string {
			switch k {
			case "XDG_STATE_HOME":
				return stateHome
			case "MEM_DB":
				return dbPath
			}
			if base == nil {
				return ""
			}
			return base(k)
		},
	}
}

func scanAll(files []string, since time.Time, horizon, nw int) ([]failureCase, error) {
	results := make([][]failureCase, len(files))
	errs := make([]error, len(files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range nw {
		wg.Go(func() {
			for i := range jobs {
				f, err := os.Open(files[i])
				if err != nil {
					errs[i] = err
					continue
				}
				sc, err := scanFailures(f, files[i])
				_ = f.Close()
				if err != nil {
					errs[i] = err
					continue
				}
				for _, ev := range sc.events {
					if !since.IsZero() && ev.Time.Before(since) {
						continue
					}
					results[i] = append(results[i], failureCase{failure: ev, lookups: sc.lookups(ev, horizon)})
				}
			}
		})
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	var out []failureCase
	for i := range files {
		if errs[i] != nil {
			return nil, fmt.Errorf("%s: %w", files[i], errs[i])
		}
		out = append(out, results[i]...)
	}
	slices.SortStableFunc(out, func(a, b failureCase) int {
		if c := a.Time.Compare(b.Time); c != 0 {
			return c
		}
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return a.call - b.call
	})
	return out, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func snapshotDB(src, dir string) (string, error) {
	si, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, env.DBName)
	if di, err := os.Stat(dst); err == nil && os.SameFile(si, di) {
		return dst, nil
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(dst + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	if _, err := os.Stat(src + "-wal"); err == nil {
		if err := copyFile(src+"-wal", dst+"-wal"); err != nil {
			return "", err
		}
	}
	return dst, nil
}

func resetStateHome(stateHome, stateDir string) error {
	mark := filepath.Join(stateHome, FailureStateMark)
	if _, err := os.Lstat(stateHome); err == nil {
		if _, err := os.Stat(mark); err != nil {
			return fmt.Errorf("%s exists and is not a replay state dir, refusing to delete it", stateHome)
		}
		if err := os.RemoveAll(stateHome); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(mark, nil, 0o600)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func RunFailures(ctx context.Context, e env.Env, cfg FailuresConfig, progress io.Writer) (FailuresReport, error) {
	rep := FailuresReport{Since: cfg.SinceText, DryRun: cfg.DryRun}
	out, err := filepath.Abs(cfg.Out)
	if err != nil {
		return rep, err
	}
	dbPath, err := filepath.Abs(cfg.DB)
	if err != nil {
		return rep, err
	}
	rep.DB = dbPath
	matched, err := globFiles(cfg.Glob, e.Home())
	if err != nil {
		return rep, err
	}
	if len(matched) == 0 {
		return rep, fmt.Errorf("no transcripts match %s", cfg.Glob)
	}
	rep.Matched = len(matched)
	var files []string
	for _, f := range matched {
		if info, err := os.Stat(f); err == nil && (cfg.Since.IsZero() || !info.ModTime().Before(cfg.Since)) {
			files = append(files, f)
		}
	}
	rep.Files = len(files)
	turns, err := loadInjections(cfg.Turns)
	if err != nil {
		return rep, fmt.Errorf("--turns: %w", err)
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		return rep, err
	}
	dbPath, err = snapshotDB(dbPath, filepath.Join(out, FailureDBDir))
	if err != nil {
		return rep, fmt.Errorf("database snapshot: %w", err)
	}
	s, err := store.Open(ctx, dbPath, store.Options{ReadOnly: true, Now: e.Now})
	if err != nil {
		return rep, err
	}
	defer func() { _ = s.Close() }()
	get := storeLookup(ctx, s)
	cases, err := scanAll(files, cfg.Since, cfg.Horizon, workers(cfg.Workers))
	if err != nil {
		return rep, err
	}
	rep.Failures = len(cases)
	cases, rep.DroppedByMax = sampleEvenly(cases, cfg.Max)
	rep.Replayed = len(cases)
	stateHome := filepath.Join(out, FailureStateDir)
	stateDir := filepath.Join(stateHome, env.DirName)
	if !cfg.DryRun {
		if err := resetStateHome(stateHome, stateDir); err != nil {
			return rep, err
		}
	}
	ef, err := os.OpenFile(filepath.Join(out, FailureEventsName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return rep, err
	}
	defer func() { _ = ef.Close() }()
	w := bufio.NewWriter(ef)
	var ms []float64
	for i, c := range cases {
		plain, pointers := injectedBefore(turns[c.Session], c.Time)
		pos, already, inel, err := splitPositives(c.lookups, plain, pointers, c.Time, get)
		if err != nil {
			return rep, err
		}
		ev := FailureEvent{
			Time: c.Time.UTC().Format(time.RFC3339Nano), Session: c.Session, Agent: c.Agent, File: c.File,
			Command: clip(c.Command, failureCommandChars), Error: lastRunes(c.Error, failureErrorChars),
			Positives: pos, AlreadyInjected: already, Ineligible: inel,
		}
		if !cfg.DryRun {
			seen, err := session.Open(stateDir, seenID(c.failure))
			if err != nil {
				return rep, err
			}
			seed := slices.Clone(plain)
			for _, k := range pointers {
				seed = append(seed, failurePointerMark+k)
			}
			if err := seen.Add(seed...); err != nil {
				return rep, err
			}
			in, err := failureInput(c.failure)
			if err != nil {
				return rep, err
			}
			var hookOut, hookErr bytes.Buffer
			args := append([]string{hook.ToolFailure, "--as-of", c.Time.UTC().Format(time.RFC3339Nano), "--no-turns-log"}, cfg.HookFlags...)
			start := time.Now()
			hook.Run(args, replayEnv(e, stateHome, dbPath, c.failure, in, &hookOut, &hookErr))
			ev.Ms = time.Since(start).Milliseconds()
			ms = append(ms, float64(ev.Ms))
			rep.Latency.Max = max(rep.Latency.Max, ev.Ms)
			ev.Injected = pointerKeys(hookOut.Bytes())
			for _, k := range ev.Injected {
				if slices.Contains(pos, k) {
					ev.Hits = append(ev.Hits, k)
				}
			}
			if progress != nil && (i+1)%failureProgress == 0 {
				_, _ = fmt.Fprintf(progress, "replayed %d of %d\n", i+1, len(cases))
			}
		}
		rep.addEvent(ev)
		ev.Injected, ev.Positives, ev.AlreadyInjected, ev.Ineligible, ev.Hits = nonNil(ev.Injected), nonNil(ev.Positives), nonNil(ev.AlreadyInjected), nonNil(ev.Ineligible), nonNil(ev.Hits)
		data, err := json.Marshal(ev)
		if err != nil {
			return rep, err
		}
		if _, err := w.Write(append(data, '\n')); err != nil {
			return rep, err
		}
		if err := w.Flush(); err != nil {
			return rep, err
		}
	}
	if q := quantiles(ms); q.N > 0 {
		rep.Latency.P50, rep.Latency.P90 = int64(q.P50), int64(q.P90)
	}
	rep.Verdict, rep.Reason = failureVerdict(rep)
	if err := ef.Close(); err != nil {
		return rep, err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return rep, err
	}
	return rep, os.WriteFile(filepath.Join(out, FailureReportName), append(data, '\n'), 0o600)
}

func (r *FailuresReport) addEvent(ev FailureEvent) {
	r.AlreadyInjected += len(ev.AlreadyInjected)
	r.Ineligible += len(ev.Ineligible)
	r.Positives += len(ev.Positives)
	if len(ev.Positives) > 0 {
		r.PositiveEvents++
	}
	r.InjectedPointers += len(ev.Injected)
	if len(ev.Injected) > 0 {
		r.InjectedEvents++
		if len(ev.Hits) == 0 {
			r.NoiseEvents++
		}
	}
	r.HitKeys += len(ev.Hits)
	if len(ev.Hits) > 0 {
		r.HitEvents++
	}
}

func printFailures(w io.Writer, r FailuresReport, out string) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	since := r.Since
	if since == "" {
		since = "the start"
	}
	p("database: %s, read from the snapshot %s\n", r.DB, filepath.Join(out, FailureDBDir, env.DBName))
	p("transcripts: %d matched, %d read (files last written before %s skipped)\n", r.Matched, r.Files, since)
	p("failures since %s: %d, replayed %d, dropped by --max %d\n", since, r.Failures, r.Replayed, r.DroppedByMax)
	p("positives: %d events with a later lookup of an eligible record, %d keys; already injected %d, ineligible %d\n",
		r.PositiveEvents, r.Positives, r.AlreadyInjected, r.Ineligible)
	if r.DryRun {
		p("hook: not called (--dry-run)\n")
	} else {
		p("hook: %d events injected %d pointers; hits %d events, %d keys; noise %d events\n", r.InjectedEvents, r.InjectedPointers, r.HitEvents, r.HitKeys, r.NoiseEvents)
		p("latency ms: p50 %d, p90 %d, max %d\n", r.Latency.P50, r.Latency.P90, r.Latency.Max)
	}
	p("verdict: %s (%s)\n", r.Verdict, r.Reason)
	p("wrote %s and %s\n", filepath.Join(out, FailureEventsName), filepath.Join(out, FailureReportName))
}

const failuresUsage = "usage: mem eval failures --transcripts GLOB --turns FILE --out DIR [--since YYYY-MM-DD] [--max N] [--horizon N] [--dry-run] [--db PATH] [-- HOOK FLAGS...]\n" +
	"       replays every failed Bash call of the transcripts (** matches any depth, subagents included) through mem hook post-tool-use-failure,\n" +
	"       one call at a time, with --as-of the failure time, a private state dir DIR/failures-state seeded with the keys the session had been given\n" +
	"       (turns.jsonl of --turns) and the HOOK FLAGS; positives are the keys of mem get in the next --horizon Bash calls, minus keys already\n" +
	"       injected in the session and records missing from the database or updated after the failure; writes DIR/events.jsonl and DIR/report.json\n" +
	"       the database is copied to DIR/db and only that copy is opened, read-only, so nothing is written next to the original;\n" +
	"       a --db that already is DIR/db/mem.db is used as it is\n" +
	"       DIR/failures-state is recreated on each run; an existing one this tool did not create is never deleted, the run fails instead\n" +
	"       --dry-run counts failures and positives without calling the hook\n"

func runFailures(args []string, e env.Env) int {
	fs := flag.NewFlagSet("mem eval failures", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(e.Stderr, failuresUsage)
		fs.PrintDefaults()
	}
	var cfg FailuresConfig
	fs.StringVar(&cfg.Glob, "transcripts", "", "Claude Code transcripts, a ** segment matches any depth")
	fs.StringVar(&cfg.Turns, "turns", "", "turns.jsonl of the hook, the injections the sessions got")
	fs.StringVar(&cfg.Out, "out", "", "output directory for events.jsonl, report.json and the private hook state")
	db := fs.String("db", e.DBFlag, "database, opened read-only")
	fs.StringVar(&cfg.SinceText, "since", "", "skip failures before local midnight of this date, YYYY-MM-DD")
	fs.IntVar(&cfg.Max, "max", DefaultFailureMax, "replay at most N failures, evenly spaced")
	fs.IntVar(&cfg.Horizon, "horizon", DefaultFailureHorizon, "Bash calls after the failure searched for mem get")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "count failures and positives, do not call the hook")
	fs.IntVar(&cfg.Workers, "workers", 0, "parallel transcript readers, 0 = min(8, CPUs)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	cfg.HookFlags = fs.Args()
	if cfg.Glob == "" || cfg.Turns == "" || cfg.Out == "" {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval failures: --transcripts, --turns and --out are required")
		return 2
	}
	if cfg.Max < 1 || cfg.Horizon < 1 || cfg.Workers < 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval failures: --max and --horizon must be at least 1, --workers not negative")
		return 2
	}
	if cfg.SinceText != "" {
		t, err := time.ParseInLocation(time.DateOnly, cfg.SinceText, time.Local)
		if err != nil {
			_, _ = fmt.Fprintf(e.Stderr, "mem eval failures: --since: %v\n", err)
			return 2
		}
		cfg.Since = t
	}
	de := e
	de.DBFlag = *db
	cfg.DB = expandHome(de.DBPath(), e.Home())
	cfg.Turns = expandHome(cfg.Turns, e.Home())
	cfg.Out = expandHome(cfg.Out, e.Home())
	rep, err := RunFailures(context.Background(), e, cfg, e.Stderr)
	if err != nil {
		return evalFail(e, err)
	}
	out, _ := filepath.Abs(cfg.Out)
	printFailures(e.Stdout, rep, out)
	return 0
}

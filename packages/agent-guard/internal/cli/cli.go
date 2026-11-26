package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"agent-guard/internal/call"
	"agent-guard/internal/claudecode"
	"agent-guard/internal/config"
	"agent-guard/internal/edit"
	"agent-guard/internal/evidence"
	"agent-guard/internal/guard"
	"agent-guard/internal/harness"
	"agent-guard/internal/journal"
	"agent-guard/internal/pi"
	"agent-guard/internal/power"
	"agent-guard/internal/replay"
	"agent-guard/internal/shell"
	"agent-guard/internal/transcript"
	"agent-guard/internal/verdict"
)

const program = "agent-guard"

const usage = `usage: ` + program + ` [--config file.json] <command>

hooks (JSON on stdin):
  hook                one hook event of the configured harness: the shell guards on shell commands
                      and the script files they run, the file checks on edits and writes with
                      shell notes on written scripts and Nix shell strings, a journal line for every confirmation
                      dialog, and with power enabled a note on battery before a heavy build and one
                      line after a tool call or with a prompt when the power source changed; Claude
                      Code results of the evidenceTools lose the REA Evidence parts this agent was
                      already shown, merge limitations that differ in one address and put a
                      multi-line result after the JSON as plain text; what was shown is
                      forgotten on PreCompact
  confine <agent>     Claude Code PreToolUse for a read-only agent: writes only under its memory dirs

power (no config needed, $AGENT_GUARD_POWER_SUPPLY overrides /sys/class/power_supply):
  power status                print the power source and the battery charge
  power wait ac|battery [-timeout D]
                              block until the machine runs on that source, polling every 2s; exit 1
                              on timeout or when waiting for battery on a machine without one

tools:
  explain '<command>'         show the commands the analysis sees, each verdict and a rewrite
  corpus [-top-only] [transcripts-dir]
                              dump every distinct shell command from the transcripts, NUL-separated,
                              subagent transcripts included
  edits [-top-only] [transcripts-dir]
                              run the file checks over every past edit and write, subagent transcripts
                              included; TSV: level, file, message, check, tool, scope, bytes. Edits are
                              applied to today's files and writes are judged as new files
  replay [-check NAME] [-full] [-assume-new] <corpus.nul>
                              run the guards over a corpus, TSV of hits on stdout: check, level, hash,
                              command
  diff <old.tsv> <new.tsv>    compare two replay outputs
  stats [-days N] [journal...]
                              count dialogs and verdicts from the hook journals
  config check                validate the configuration and load every grammar
  journal import <legacy.jsonl>...
                              move legacy journal lines into the configured journal, stamped with the
                              configured harness; reruns append only new lines
`

type Env struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Getenv    func(string) string
	Now       func() time.Time
	Getwd     func() (string, error)
	PowerRoot string
}

func System() Env {
	root := os.Getenv("AGENT_GUARD_POWER_SUPPLY")
	if root == "" {
		root = power.DefaultRoot
	}
	return Env{
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Getenv:    os.Getenv,
		Now:       time.Now,
		Getwd:     os.Getwd,
		PowerRoot: root,
	}
}

type run struct {
	env        Env
	configPath string
	cfg        *config.Config
}

type exit int

func Run(args []string, env Env) int {
	fs := flag.NewFlagSet(program, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	configPath := fs.String("config", env.Getenv("AGENT_GUARD_CONFIG"), "guard configuration JSON")
	fs.Usage = func() { _, _ = fmt.Fprint(env.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 1
	}
	r := &run{env: env, configPath: *configPath}
	code, err := r.dispatch(rest)
	var e exit
	if errors.As(err, &e) {
		return int(e)
	}
	if err != nil {
		_, _ = fmt.Fprintln(env.Stderr, program+":", err)
		return 1
	}
	return code
}

func (e exit) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func (r *run) config() (*config.Config, error) {
	if r.cfg != nil {
		return r.cfg, nil
	}
	cfg, err := config.Load(r.configPath, r.env.Getenv)
	if err != nil {
		_, _ = fmt.Fprintln(r.env.Stderr, program+": config:", err)
		return nil, exit(1)
	}
	r.cfg = cfg
	return cfg, nil
}

type codec struct {
	hook   func(r *run, cfg *config.Config) int
	source func(cfg *config.Config) transcript.Source
}

var codecs = map[harness.ID]codec{
	harness.ClaudeCode: {(*run).claudeCode, func(*config.Config) transcript.Source { return claudecode.Source{} }},
	harness.Pi:         {(*run).piCodec, func(cfg *config.Config) transcript.Source { return pi.Source{Home: cfg.Home} }},
}

func (r *run) codec() (*config.Config, codec, error) {
	cfg, err := r.config()
	if err != nil {
		return nil, codec{}, err
	}
	c, ok := codecs[cfg.Spec.ID]
	if !ok {
		return nil, codec{}, fmt.Errorf("no codec for harness %s", cfg.Spec.ID)
	}
	return cfg, c, nil
}

func (r *run) dispatch(args []string) (int, error) {
	switch args[0] {
	case "hook":
		return r.hook()
	case "confine":
		if len(args) < 2 || args[1] == "" {
			_, _ = fmt.Fprintln(r.env.Stderr, program+": confine needs the agent name")
			return 2, nil
		}
		return r.confine(args[1])
	case "config":
		if len(args) != 2 || args[1] != "check" {
			return 0, errors.New("config takes check")
		}
		cfg, err := r.config()
		if err != nil {
			return 0, err
		}
		return 0, cfg.Check()
	case "stats":
		return r.stats(args[1:])
	case "journal":
		if len(args) < 3 || args[1] != "import" {
			return 0, errors.New("journal import takes the legacy journals to move")
		}
		return r.journalImport(args[2:])
	case "explain":
		if len(args) < 2 {
			return 0, errors.New("explain needs a command")
		}
		return r.explain(args[1])
	case "corpus":
		return r.corpus(args[1:])
	case "edits":
		return r.edits(args[1:])
	case "replay":
		return r.replay(args[1:])
	case "diff":
		if len(args) < 3 {
			return 0, errors.New("diff needs two replay outputs")
		}
		return 0, replay.Diff(args[1], args[2], r.env.Stdout)
	case "power":
		return r.powerCommand(args[1:])
	}
	_, _ = fmt.Fprint(r.env.Stderr, usage)
	return 1, nil
}

func (r *run) engine(cfg *config.Config) *guard.Engine {
	return &guard.Engine{
		Checks:  cfg.Checks(),
		Edit:    cfg.EditOptions(),
		Memory:  cfg.Memory(),
		Now:     r.env.Now,
		Journal: cfg.Journal,
	}
}

func (r *run) hookEngine(cfg *config.Config) *guard.Engine {
	e := r.engine(cfg)
	e.ReadScript = edit.ReadFile
	if !cfg.Power.Enable {
		return e
	}
	heavy, err := power.Parse(cfg.Power.HeavyCommands)
	if err != nil {
		_, _ = fmt.Fprintln(r.env.Stderr, program+": config: power:", err)
		return e
	}
	e.Power = &guard.Power{
		Probe:  &power.Probe{Root: r.env.PowerRoot},
		Heavy:  heavy,
		Memory: power.Memory{Dir: cfg.PowerState(), Now: r.env.Now},
	}
	return e
}

func (r *run) hook() (int, error) {
	cfg, c, err := r.codec()
	if err != nil {
		return 0, err
	}
	return c.hook(r, cfg), nil
}

var (
	claudeObserved = map[string]string{"PostToolUse": journal.PostTool, "UserPromptSubmit": journal.UserPrompt}
	piObserved     = map[string]string{pi.ToolResult: journal.PostTool, pi.Prompt: journal.UserPrompt}
)

func observe(e *guard.Engine, c call.Call, hook string) guard.Decision {
	d := e.PowerChange(c)
	if d.Outcome != verdict.Silent.Outcome() {
		e.Record(c, hook, d)
	}
	return d
}

func (r *run) failed(e *guard.Engine, c call.Call, label string, err error) {
	_, _ = fmt.Fprintln(r.env.Stderr, program+": bad hook input:", err)
	e.Record(c, label, guard.Decision{Outcome: verdict.Failed})
}

func (r *run) evidence(cfg *config.Config) evidence.Memory {
	return evidence.Memory{Dir: cfg.EvidenceState(), Now: r.env.Now}
}

func (r *run) compactEvidence(cfg *config.Config, e *guard.Engine, c call.Call, response json.RawMessage) (string, bool) {
	if !evidence.Matches(cfg.EvidenceTools, c.Tool) {
		return "", false
	}
	var text string
	if json.Unmarshal(response, &text) != nil {
		return "", false
	}
	out, ok, err := r.evidence(cfg).Compact(c.StateKey(), text)
	if err != nil {
		_, _ = fmt.Fprintln(r.env.Stderr, program+": evidence:", err)
		return "", false
	}
	if ok {
		e.Record(c, journal.Evidence, guard.Decision{Outcome: verdict.Rewrite, Notes: []string{fmt.Sprintf("%d -> %d bytes", len(text), len(out))}})
	}
	return out, ok
}

func (r *run) claudeCode(cfg *config.Config) int {
	e := r.hookEngine(cfg)
	in, err := claudecode.Read(r.env.Stdin)
	if err == nil && in.HookEventName == "PreCompact" {
		key := call.Call{Harness: cfg.Spec.ID, Session: in.SessionID}.StateKey()
		if err := r.evidence(cfg).Forget(key); err != nil {
			_, _ = fmt.Fprintln(r.env.Stderr, program+": evidence:", err)
		}
		return 0
	}
	if hook, ok := claudeObserved[in.HookEventName]; err == nil && ok {
		c, _ := claudecode.Decode(in)
		c.Harness = cfg.Spec.ID
		d := observe(e, c, hook)
		if in.HookEventName == "PostToolUse" {
			if out, ok := r.compactEvidence(cfg, e, c, in.ToolResponse); ok {
				return claudecode.Write(r.env.Stdout, r.env.Stderr, claudecode.PostTool(d, out))
			}
		}
		return claudecode.Write(r.env.Stdout, r.env.Stderr, claudecode.Context(in.HookEventName, d))
	}
	var c call.Call
	if err == nil {
		c, err = claudecode.Decode(in)
	}
	c.Harness = cfg.Spec.ID
	if err == nil && in.HookEventName != "PreToolUse" && in.HookEventName != "PermissionRequest" {
		err = fmt.Errorf("unsupported hook_event_name %q", in.HookEventName)
	}
	if err != nil {
		r.failed(e, c, "hook", err)
		return 1
	}
	if in.HookEventName == "PermissionRequest" {
		e.Record(c, journal.PermissionRequest, guard.Decision{Outcome: verdict.Dialog})
		return 0
	}
	label := journal.PreEdit
	switch c.Kind {
	case call.Shell:
		if c.Command == "" {
			return 0
		}
		label = journal.PreBash
	case call.Edit, call.Write:
	default:
		return 0
	}
	d := e.PreTool(c)
	e.Record(c, label, d)
	return claudecode.Write(r.env.Stdout, r.env.Stderr, claudecode.Encode(d, in.ToolInput))
}

func (r *run) piCodec(cfg *config.Config) int {
	e := r.hookEngine(cfg)
	p, err := pi.Read(r.env.Stdin)
	if hook, ok := piObserved[p.Event]; err == nil && ok {
		c := pi.Envelope(p)
		c.Harness = cfg.Spec.ID
		return pi.Write(r.env.Stdout, pi.Encode(observe(e, c, hook)))
	}
	var c call.Call
	guarded := false
	if err == nil {
		switch p.Event {
		case pi.ToolCall:
			c, guarded, err = pi.Decode(p, cfg.Home)
		case pi.DialogResult:
			c = pi.Envelope(p)
			c.Harness = cfg.Spec.ID
			outcome := verdict.Declined
			if p.Approved {
				outcome = verdict.Approved
			}
			e.Record(c, journal.DialogResult, guard.Decision{Outcome: outcome})
			return pi.Write(r.env.Stdout, pi.Output{})
		default:
			err = fmt.Errorf("unsupported event %q", p.Event)
		}
	}
	c.Harness = cfg.Spec.ID
	if err != nil {
		r.failed(e, c, "hook", err)
		return 1
	}
	if !guarded || (c.Kind == call.Shell && c.Command == "") {
		return pi.Write(r.env.Stdout, pi.Output{})
	}
	hook := journal.PreEdit
	if c.Kind == call.Shell {
		hook = journal.PreBash
	}
	d := e.PreTool(c)
	e.Record(c, hook, d)
	out := pi.Encode(d)
	if d.Outcome == verdict.Ask.Outcome() {
		if !p.HasUI {
			e.Record(c, journal.PermissionRequest, guard.Decision{Outcome: verdict.Deny.Outcome()})
			return pi.Write(r.env.Stdout, pi.NoUI(d))
		}
		e.Record(c, journal.PermissionRequest, guard.Decision{Outcome: verdict.Dialog})
	}
	return pi.Write(r.env.Stdout, out)
}

func (r *run) powerCommand(args []string) (int, error) {
	const how = "power takes status, or wait ac|battery [-timeout D]"
	if len(args) == 0 {
		return 0, errors.New(how)
	}
	read := func() power.State { return power.Read(r.env.PowerRoot) }
	switch args[0] {
	case "status":
		if len(args) != 1 {
			return 0, errors.New(how)
		}
		_, _ = fmt.Fprintln(r.env.Stdout, read())
		return 0, nil
	case "wait":
		fs := flag.NewFlagSet("power wait", flag.ContinueOnError)
		fs.SetOutput(r.env.Stderr)
		timeout := fs.Duration("timeout", 0, "give up after this long, 0 waits forever")
		var targets []string
		rest := args[1:]
		for {
			if err := fs.Parse(rest); err != nil {
				return 2, nil
			}
			if fs.NArg() == 0 {
				break
			}
			targets, rest = append(targets, fs.Arg(0)), fs.Args()[1:]
		}
		if len(targets) != 1 || (targets[0] != "ac" && targets[0] != "battery") {
			_, _ = fmt.Fprintln(r.env.Stderr, program+":", how)
			return 2, nil
		}
		ctx := context.Background()
		if *timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, *timeout)
			defer cancel()
		}
		s, err := power.Wait(ctx, read, targets[0] == "ac", 2*time.Second)
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("timed out after %s", *timeout)
		}
		if err != nil {
			_, _ = fmt.Fprintf(r.env.Stderr, "%s: power wait %s: %v; now %s\n", program, targets[0], err, s)
			return 1, nil
		}
		_, _ = fmt.Fprintln(r.env.Stdout, s)
		return 0, nil
	}
	return 0, errors.New(how)
}

func (r *run) confine(agent string) (int, error) {
	cfg, err := r.config()
	if err != nil {
		return 2, nil
	}
	e := r.engine(cfg)
	in, err := claudecode.Read(r.env.Stdin)
	var c call.Call
	if err == nil {
		c, err = claudecode.Decode(in)
	}
	c.Harness = cfg.Spec.ID
	if err != nil {
		r.failed(e, c, journal.AgentMemoryWrite, err)
		return 2, nil
	}
	d := e.Confine(c, claudecode.ConfineRoots(agent, claudecode.ProjectDir(r.env.Getenv, c.Cwd), cfg.Home))
	e.Record(c, journal.AgentMemoryWrite, d)
	return claudecode.Write(r.env.Stdout, r.env.Stderr, claudecode.Encode(d, in.ToolInput)), nil
}

func (r *run) stats(args []string) (int, error) {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(r.env.Stderr)
	days := fs.Int("days", 7, "count the last N days")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	paths := fs.Args()
	var fallback harness.ID
	if len(paths) == 0 {
		cfg, err := r.config()
		if err != nil {
			return 0, err
		}
		if cfg.Journal == "" {
			return 0, errors.New("stats needs a journal: pass its path or set journal in the config")
		}
		paths, fallback = []string{cfg.Journal}, cfg.Spec.ID
	}
	var entries []journal.Entry
	skipped := 0
	for _, p := range paths {
		got, n, err := journal.Read(p)
		if err != nil {
			return 0, err
		}
		entries, skipped = append(entries, got...), skipped+n
	}
	s := journal.Summarize(entries, r.env.Now().AddDate(0, 0, -*days), fallback)
	s.Skipped = skipped
	return 0, s.Write(r.env.Stdout)
}

func (r *run) journalImport(paths []string) (int, error) {
	cfg, err := r.config()
	if err != nil {
		return 0, err
	}
	if cfg.Journal == "" {
		return 0, errors.New("journal import needs journal in the config")
	}
	for _, p := range paths {
		st, err := journal.Import(cfg.Journal, p, journal.ImportOptions{Harness: cfg.Spec.ID, Now: r.env.Now, Settle: time.Second})
		if err != nil {
			return 0, fmt.Errorf("%s: %w", p, err)
		}
		_, _ = fmt.Fprintf(r.env.Stdout, "%s: %s\n", p, st)
	}
	return 0, nil
}

func (r *run) transcripts(name string, args []string) (*config.Config, transcript.Source, string, bool, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(r.env.Stderr)
	topOnly := fs.Bool("top-only", false, "skip subagent and workflow transcripts")
	if err := fs.Parse(args); err != nil {
		return nil, nil, "", false, exit(2)
	}
	if fs.NArg() > 1 {
		return nil, nil, "", false, fmt.Errorf("%s takes flags first and at most one transcripts dir", name)
	}
	cfg, c, err := r.codec()
	if err != nil {
		return nil, nil, "", false, err
	}
	dir := cfg.Transcripts
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	if dir == "" {
		return nil, nil, "", false, fmt.Errorf("%s needs a transcripts dir: pass it or set transcripts in the config", name)
	}
	return cfg, c.source(cfg), dir, *topOnly, nil
}

func (r *run) summary(head string, ts *transcript.Stats, counts map[string]int) {
	_, _ = fmt.Fprintln(r.env.Stderr, head)
	if ts != nil {
		_, _ = fmt.Fprintf(r.env.Stderr, "%d transcripts, %d unreadable, %d undecodable lines\n", ts.Files, ts.Unreadable, ts.Skipped)
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(r.env.Stderr, "%8d  %s\n", counts[k], k)
	}
}

func (r *run) corpus(args []string) (int, error) {
	_, src, dir, topOnly, err := r.transcripts("corpus", args)
	if err != nil {
		return 0, err
	}
	n, ts, err := replay.Corpus(src, dir, topOnly, r.env.Stdout)
	if err != nil {
		return 0, err
	}
	r.summary(fmt.Sprintf("%d distinct commands", n), &ts, nil)
	return 0, nil
}

func (r *run) edits(args []string) (int, error) {
	cfg, src, dir, topOnly, err := r.transcripts("edits", args)
	if err != nil {
		return 0, err
	}
	e := r.engine(cfg)
	e.Journal, e.AssumeNew, e.Edit.Previous = "", true, edit.Missing
	st, err := replay.Edits(src, dir, topOnly, e, r.env.Stdout)
	if err != nil {
		return 0, err
	}
	r.summary(fmt.Sprintf("%d edits, %d with a verdict", st.Edits, st.Hits), &st.Stats, st.Verdicts)
	return 0, nil
}

func (r *run) replay(args []string) (int, error) {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(r.env.Stderr)
	check := fs.String("check", "", "only this check")
	full := fs.Bool("full", false, "print the whole command, NUL-separated, instead of TSV")
	assumeNew := fs.Bool("assume-new", false, "treat every file a command creates as not existing yet")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	if fs.NArg() < 1 {
		return 0, errors.New("replay needs a corpus file")
	}
	cfg, err := r.config()
	if err != nil {
		return 0, err
	}
	e := r.engine(cfg)
	e.Journal, e.AssumeNew = "", *assumeNew
	st, err := replay.Run(fs.Arg(0), e, replay.Options{Check: *check, Full: *full}, r.env.Stdout)
	if err != nil {
		return 0, err
	}
	r.summary(fmt.Sprintf("%d commands", st.Commands), nil, st.Counts)
	return 0, nil
}

func (r *run) explain(command string) (int, error) {
	cfg, err := r.config()
	if err != nil {
		return 0, err
	}
	w := r.env.Stdout
	a := shell.Analyze(command)
	for _, u := range a.Unparsed {
		_, _ = fmt.Fprintf(w, "unparsed [%s d%d %s]: %v\n", u.Kind, u.Depth, u.Reason, u.Err)
	}
	for _, c := range a.Commands {
		flags := ""
		if c.Origin != shell.Parsed {
			flags += " " + c.Origin.String()
		}
		if c.Background {
			flags += " background"
		}
		if c.Detached {
			flags += " detached"
		}
		if c.InPipeline {
			flags += " pipeline"
		}
		_, _ = fmt.Fprintf(w, "[%s d%d]%s %q\n", c.Kind, c.Depth, flags, c.Argv)
		if c.Heredoc != "" {
			_, _ = fmt.Fprintf(w, "    heredoc: %q\n", c.Heredoc)
		}
		if c.Stdin != "" {
			_, _ = fmt.Fprintf(w, "    stdin: %q\n", c.Stdin)
		}
	}
	cwd, _ := r.env.Getwd()
	e := r.engine(cfg)
	e.Journal, e.ReadScript = "", edit.ReadFile
	d := e.PreTool(call.Call{Kind: call.Shell, Command: command, Cwd: cwd, Session: "explain"})
	for _, v := range d.Verdicts {
		_, _ = fmt.Fprintf(w, "%s: %s\n    %s\n", v.Level, v.Check, v.Message)
	}
	if d.Outcome == verdict.Rewrite {
		_, _ = fmt.Fprintf(w, "rewrite: %s\n", d.Command)
	}
	if cfg.Power.Enable {
		heavy, err := power.Parse(cfg.Power.HeavyCommands)
		if err != nil {
			return 0, err
		}
		if p, ok := power.Heavy(a.Commands, heavy); ok {
			_, _ = fmt.Fprintf(w, "heavy on battery: %s\n", p.Text)
		}
	}
	return 0, nil
}

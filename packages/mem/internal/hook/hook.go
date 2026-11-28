package hook

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"mem/internal/env"
	"mem/internal/pending"
	"mem/internal/recall"
	"mem/internal/session"
	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	SessionStart     = "session-start"
	UserPromptSubmit = "user-prompt-submit"
	PreToolUse       = "pre-tool-use"
	SubagentStart    = "subagent-start"
	ToolFailure      = "post-tool-use-failure"

	LogName       = statefile.HookLogName
	maxInput      = 64 << 20
	maxLogSize    = 1 << 20
	maxLogDetail  = statefile.HookLogDetail
	busyTimeout   = 500 * time.Millisecond
	pruneAge      = 7 * 24 * time.Hour
	statusError   = "error"
	statusSkipped = "skipped"
	maxToolName   = 128
)

var eventNames = map[string]string{
	SessionStart:     "SessionStart",
	UserPromptSubmit: "UserPromptSubmit",
	PreToolUse:       "PreToolUse",
	SubagentStart:    "SubagentStart",
	ToolFailure:      "PostToolUseFailure",
}

const usage = `usage: mem hook <event> [flags]

events (Claude Code command hooks, hook JSON on stdin, hook JSON on stdout):
  session-start        expire state records, emit the usage header, core records of the cwd project and the host and a topic map
                       of title keywords (--map-chars N, --map-keywords N, --no-usage); with --embed-url or MEM_EMBED_URL set it opens and closes a TCP connection to the
                       embedder (100 ms limit) so a socket-activated backend starts early, likewise the reranker with --rerank-url or MEM_RERANK_URL
  user-prompt-submit   emit records related to the prompt; --vec-pool reg|fused-reg|all|fused-all with --embed-url and --embed-model
                       adds a query embedding (limit --embed-timeout), any embedder failure falls back to the lexical path
                       --reg-pointer-cos / --log-pointer-cos add a second tier: records below the gate but at or above that cosine
                       are listed as one-line pointers (--pointer-take per kind), never repeated within a session
                       --rerank-url URL (env MEM_RERANK_URL) sends the top --rerank-candidates of a vector pool to a cross-encoder
                       and gates on its score: --rerank-text for full records (at most --rerank-take), --rerank-pointer for pointers;
                       any reranker failure keeps the cosine gate for that prompt (limit --rerank-timeout, at most 3/4 of the budget left)
                       --shadow-vec-pool POOL appends what that pool would inject to $XDG_STATE_HOME/agent-memory/shadow.jsonl (prompt hash, no text), never injects it
                       every prompt that reaches the search appends a line to $XDG_STATE_HOME/agent-memory/turns.jsonl for mem ui: the prompt
                       hash, its first 100 chars, the injected keys with their cosines, the embed outcome and the timings (--no-turns-log: off)
                       text that starts with <agent-message, <task-notification, <system-reminder, <local-command- or a --skip-prefix
                       gets nothing and a turn with status skipped and source naming the marker
  pre-tool-use         deny file writes into the memory store, emit records whose trigger matches the call; an injection appends the tool
                       name and the keys to turns.jsonl (--no-turns-log: off); a trigger record is injected once per agent
                       (--trigger-once session: once per session, its subagents included)
                       for the Agent and Task tools (not subagent_type fork) it also runs the prompt recall on tool_input.prompt, with the
                       recall, --vec-pool and rerank flags of user-prompt-submit (--recall-max-chars for --max-chars), and queues the
                       result for the subagent under the session and the subagent_type (general-purpose when omitted)
                       --agent-recall: at the first tool call of an agent whose transcript lies under subagents/workflows/ (spawned by
                       the Workflow tool, so nothing was queued) run that recall on the task text of the transcript and emit it once
  post-tool-use-failure
                       PostToolUseFailure: recall on the failed call (its command or path plus the tail of the error text) with the
                       recall, --vec-pool and rerank flags of user-prompt-submit, and emit at most 3 one-line pointers, never a record
                       in full; a pointer is listed once per session and the same failure text is looked up once; interrupts and
                       errors without text get nothing
  subagent-start       dequeue the oldest queued recall of the session for agent_type, entries older than --pending-ttl are dropped,
                       and emit it plus a topic map (--map-chars N, --map-keywords N, --no-map); with an empty queue only the map; the
                       hook input carries no prompt, so a Workflow agent, which spawns without an Agent tool call, gets the map only

The project is the origin remote path without its host, lowercased (github.com/Example-Org/infra: example-org/infra), else the
git root with $HOME as ~; worktrees resolve to their main repository. Every event matches global records and records of the
project or the host; --strict-host keeps a record with host scopes to those hosts, --strict-project likewise for projects.

Every turns.jsonl line also carries harness (--harness NAME, default claude-code), host, project, cwd and the subagent type; a tool line
carries a 160-char preview of the tool input and the trigger regexes that fired; session-start and failed hooks append a line too.

A hook always exits 0; failures and budget overruns leave stdout empty and add a line to
$XDG_STATE_HOME/agent-memory/hook.log.
`

type Input struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`
	AgentID        string          `json:"agent_id"`
	AgentType      string          `json:"agent_type"`
	Source         string          `json:"source"`
	Prompt         string          `json:"prompt"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	Error          string          `json:"error"`
	IsInterrupt    bool            `json:"is_interrupt"`
}

type Output struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

type config struct {
	budget      time.Duration
	maxChars    int
	recall      recall.Settings
	short       int
	tail        int
	vec         vectorConfig
	mapChars    int
	mapKeywords int
	noUsage     bool
	noTurns     bool
	noMap       bool
	pendingTTL  time.Duration
	strict      bool
	strictHost  bool
	triggerOnce string
	harness     string
	agentRecall bool
	asOf        time.Time

	skipPrefixes multiFlag
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

type result struct {
	session string
	out     *Output
	seen    *session.Seen
	keys    []string
	status  string
	detail  []string
	err     error
	after   func(context.Context)
	turn    *statefile.Turn
	also    *marks
	extra   []statefile.Turn
	claim   string
}

func (r *result) release(e env.Env) {
	if r.claim == "" {
		return
	}
	if err := session.Unclaim(e.StateDir(), r.claim, session.RecallClaim); err != nil {
		r.note("agent recall: %v", err)
	}
}

func (r *result) note(format string, a ...any) {
	r.detail = append(r.detail, fmt.Sprintf(format, a...))
}

func (r *result) embedded(outcome string, took time.Duration) {
	if r.turn != nil {
		r.turn.Embed, r.turn.EmbedMs = outcome, took.Milliseconds()
	}
}

func writeTurn(e env.Env, cfg config, event string, r result, status string, elapsed time.Duration, in *Input) {
	if cfg.noTurns {
		return
	}
	base := r.turn
	if base == nil && status == statusError && in != nil {
		base = errorTurn(event, e, *in)
	}
	if base == nil {
		return
	}
	t := *base
	if in != nil {
		stampTurn(&t, e, cfg, *in)
	}
	t.Status, t.TotalMs = status, elapsed.Milliseconds()
	if status == statusError {
		t.Strong, t.Pointers, t.Lexical, t.Keys, t.Chars = nil, nil, nil, nil, 0
	}
	if err := statefile.Append(e.StateDir(), statefile.TurnsName, statefile.MaxTurnsSize, t); err != nil {
		writeLog(e, event, r.session, statusError, "turns: "+err.Error(), 0)
	}
}

func Run(args []string, e env.Env) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(e.Stderr, usage)
		return 0
	}
	event := args[0]
	if _, ok := eventNames[event]; !ok {
		_, _ = fmt.Fprintf(e.Stderr, "mem hook: unknown event %q\n%s", event, usage)
		writeLog(e, event, "", "error", "unknown event", 0)
		return 0
	}
	cfg, ok := parseFlags(event, args[1:], &e)
	if !ok {
		writeLog(e, event, "", "error", "bad flags", 0)
		return 0
	}
	if cfg.vec.shadowErr != nil {
		writeLog(e, event, "", "error", "shadow off: "+cfg.vec.shadowErr.Error(), 0)
	}
	if cfg.vec.rerank.err != nil {
		writeLog(e, event, "", "error", "rerank off: "+cfg.vec.rerank.err.Error(), 0)
	}
	if cfg.vec.off != nil {
		writeLog(e, event, "", "error", "vector off: "+cfg.vec.off.Error(), 0)
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.budget)
	defer cancel()
	done := make(chan result, 1)
	var seenIn atomic.Pointer[Input]
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- result{err: fmt.Errorf("panic: %v", p)}
			}
		}()
		done <- handle(ctx, event, cfg, e, &seenIn)
	}()
	var r result
	select {
	case r = <-done:
	case <-ctx.Done():
		r = result{err: ctx.Err()}
		select {
		case late := <-done:
			r.claim = late.claim
		default:
		}
	}
	elapsed := time.Since(start)
	if r.err != nil {
		r.release(e)
		msg := r.err.Error()
		if errors.Is(r.err, context.DeadlineExceeded) {
			msg = fmt.Sprintf("over the %s budget", cfg.budget)
		}
		writeLog(e, event, r.session, "error", strings.Join(append(r.detail, msg), "; "), elapsed)
		writeTurn(e, cfg, event, r, statusError, elapsed, seenIn.Load())
		return 0
	}
	if r.out != nil {
		data, err := encode(*r.out)
		if err != nil {
			r.release(e)
			writeLog(e, event, r.session, "error", err.Error(), elapsed)
			writeTurn(e, cfg, event, r, statusError, elapsed, seenIn.Load())
			return 0
		}
		_, _ = fmt.Fprintln(e.Stdout, data)
		if !cfg.noTurns {
			for _, t := range r.extra {
				if err := statefile.Append(e.StateDir(), statefile.TurnsName, statefile.MaxTurnsSize, t); err != nil {
					writeLog(e, event, r.session, statusError, "turns: "+err.Error(), 0)
				}
			}
		}
		if r.seen != nil && len(r.keys) > 0 {
			if err := r.seen.Add(r.keys...); err != nil {
				r.note("session state: %v", err)
			}
		}
		if r.also != nil && len(r.also.keys) > 0 {
			if err := r.also.seen.Add(r.also.keys...); err != nil {
				r.note("session state: %v", err)
			}
		}
	}
	status := r.status
	if status == "" {
		status = "none"
	}
	if r.out != nil || event != PreToolUse || len(r.detail) > 0 {
		writeLog(e, event, r.session, status, strings.Join(r.detail, "; "), elapsed)
	}
	if r.after != nil {
		runAfter(ctx, e, event, r)
	}
	writeTurn(e, cfg, event, r, status, elapsed, seenIn.Load())
	return 0
}

func runAfter(ctx context.Context, e env.Env, event string, r result) {
	detach(e.Stdout)
	detach(e.Stderr)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if p := recover(); p != nil {
				writeLog(e, event, r.session, "error", fmt.Sprintf("after output: panic: %v", p), 0)
			}
		}()
		r.after(ctx)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func addRecallFlags(fs *flag.FlagSet, cfg *config, maxCharsFlag string) {
	d := recall.DefaultSettings()
	fs.IntVar(&cfg.recall.MaxChars, maxCharsFlag, d.MaxChars, "budget of the emitted context in chars")
	fs.IntVar(&cfg.recall.Limit, "limit", d.Limit, "most records emitted")
	fs.IntVar(&cfg.recall.Pool, "pool", d.Pool, "fused hits examined by the gate")
	fs.Float64Var(&cfg.recall.MinCover, "min-cover", d.MinCover, "least share of the query stems found in a record")
	fs.IntVar(&cfg.recall.MinShared, "min-shared", d.MinShared, "least stems a record shares with the query")
	fs.IntVar(&cfg.recall.MinTopic, "min-topic", d.MinTopic, "least stems of the record key and title found in the query")
	fs.BoolVar(&cfg.recall.Lead, "lead", d.Lead, "accept the top hit without --min-cover when it ranks first in both the words and the trigram table and the runner-up has no trigram match")
	fs.IntVar(&cfg.recall.LeadShared, "lead-shared", d.LeadShared, "least shared stems of a top hit taken by --lead")
	fs.Func("as-of", "consider only records not updated after this RFC3339 time; for replays of past sessions", func(v string) error {
		t, err := time.Parse(time.RFC3339, v)
		cfg.asOf = t
		return err
	})
}

func parseFlags(event string, args []string, e *env.Env) (config, bool) {
	cfg := config{}
	fs := flag.NewFlagSet("mem hook "+event, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	db := fs.String("db", e.DBFlag, "database path")
	fs.DurationVar(&cfg.budget, "budget", 2*time.Second, "time budget; past it the hook prints nothing")
	fs.BoolVar(&cfg.strict, "strict-project", false, "a record with project scopes matches only inside one of those projects")
	fs.BoolVar(&cfg.strictHost, "strict-host", false, "a record with host scopes matches only on one of those hosts")
	fs.StringVar(&cfg.harness, "harness", defaultHarness, "name of the agent harness written to turns.jsonl")
	switch event {
	case SessionStart:
		fs.IntVar(&cfg.maxChars, "max-chars", 2000, "budget of the emitted context in chars")
		fs.IntVar(&cfg.mapChars, "map-chars", 2500, "budget of the topic map in chars, 0 for no map")
		fs.IntVar(&cfg.mapKeywords, "map-keywords", store.MapKeywords, "title keywords per topic in the topic map, 0 for none")
		fs.BoolVar(&cfg.noUsage, "no-usage", false, "leave the usage header out")
		fs.StringVar(&cfg.vec.url, "embed-url", e.Getenv("MEM_EMBED_URL"), "embedding server base URL; only connected to once to wake it (env MEM_EMBED_URL)")
		fs.StringVar(&cfg.vec.rerank.url, "rerank-url", e.Getenv("MEM_RERANK_URL"), "reranker base URL; only connected to once to wake it (env MEM_RERANK_URL)")
		fs.BoolVar(&cfg.noTurns, "no-turns-log", false, "append nothing to $XDG_STATE_HOME/agent-memory/turns.jsonl")
	case UserPromptSubmit:
		addRecallFlags(fs, &cfg, "max-chars")
		fs.IntVar(&cfg.short, "short", 40, "prompts shorter than this many chars get the tail of the last assistant message")
		fs.IntVar(&cfg.tail, "tail", 600, "chars taken from the end of the last assistant message")
		addVectorFlags(fs, &cfg, *e)
		fs.BoolVar(&cfg.noTurns, "no-turns-log", false, "append nothing to $XDG_STATE_HOME/agent-memory/turns.jsonl")
		fs.Var(&cfg.skipPrefixes, "skip-prefix", "a prompt starting with this text is not typed by the user (repeatable); like <agent-message, <task-notification, <system-reminder and <local-command- it gets no records and a skipped turn")
	case PreToolUse:
		fs.IntVar(&cfg.maxChars, "max-chars", 2500, "budget of the emitted context in chars")
		fs.StringVar(&cfg.triggerOnce, "trigger-once", triggerOnceAgent, "a trigger record is injected once per agent or once per session (shared by its subagents): agent or session")
		fs.BoolVar(&cfg.noTurns, "no-turns-log", false, "append nothing to $XDG_STATE_HOME/agent-memory/turns.jsonl")
		addRecallFlags(fs, &cfg, "recall-max-chars")
		addVectorFlags(fs, &cfg, *e)
		fs.BoolVar(&cfg.agentRecall, "agent-recall", false, "an agent that got no queued recall, such as one spawned by the Workflow tool, gets the recall of its task at its first tool call; the task is read from the agent transcript next to transcript_path")
	case ToolFailure:
		addRecallFlags(fs, &cfg, "max-chars")
		addVectorFlags(fs, &cfg, *e)
		fs.BoolVar(&cfg.noTurns, "no-turns-log", false, "append nothing to $XDG_STATE_HOME/agent-memory/turns.jsonl")
	case SubagentStart:
		fs.IntVar(&cfg.mapChars, "map-chars", 1500, "budget of the topic map in chars, 0 for no map")
		fs.IntVar(&cfg.mapKeywords, "map-keywords", store.MapKeywords, "title keywords per topic in the topic map, 0 for none")
		fs.BoolVar(&cfg.noMap, "no-map", false, "leave the topic map out")
		fs.DurationVar(&cfg.pendingTTL, "pending-ttl", defaultPendingTTL, "age past which a queued recall no longer belongs to a starting subagent")
		fs.BoolVar(&cfg.noTurns, "no-turns-log", false, "append nothing to $XDG_STATE_HOME/agent-memory/turns.jsonl")
	}
	if err := fs.Parse(args); err != nil {
		return cfg, false
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem hook %s: unexpected arguments %q\n", event, fs.Args())
		return cfg, false
	}
	if event == PreToolUse && cfg.triggerOnce != triggerOnceSession && cfg.triggerOnce != triggerOnceAgent {
		_, _ = fmt.Fprintf(e.Stderr, "mem hook %s: --trigger-once must be session or agent, not %q\n", event, cfg.triggerOnce)
		return cfg, false
	}
	if event == SubagentStart && (cfg.pendingTTL <= 0 || cfg.pendingTTL > pending.Retention) {
		_, _ = fmt.Fprintf(e.Stderr, "mem hook %s: --pending-ttl must lie in (0,%s], not %s\n", event, pending.Retention, cfg.pendingTTL)
		return cfg, false
	}
	if cfg.budget <= 0 {
		cfg.budget = time.Nanosecond
	}
	if err := cfg.vec.finish(); err != nil {
		if event != PreToolUse {
			_, _ = fmt.Fprintf(e.Stderr, "mem hook %s: %v\n", event, err)
			return cfg, false
		}
		cfg.vec = vectorConfig{off: err, settings: recall.VectorSettings{PointerTake: cfg.vec.settings.PointerTake}}
	}
	if event == UserPromptSubmit || event == PreToolUse || event == ToolFailure {
		cfg.vec.rerank.finish()
	}
	e.DBFlag = *db
	return cfg, true
}

func handle(ctx context.Context, event string, cfg config, e env.Env, seenIn *atomic.Pointer[Input]) result {
	if err := ctx.Err(); err != nil {
		return result{err: err}
	}
	in, err := readInput(e.Stdin)
	if err != nil {
		return result{err: fmt.Errorf("hook input: %w", err)}
	}
	seenIn.Store(&in)
	if in.HookEventName != "" && in.HookEventName != eventNames[event] {
		return result{session: in.SessionID, err: fmt.Errorf("hook_event_name %q sent to %s", in.HookEventName, event)}
	}
	var r result
	switch event {
	case SessionStart:
		r = sessionStart(ctx, cfg, e, in)
	case UserPromptSubmit:
		r = userPromptSubmit(ctx, cfg, e, in)
	case SubagentStart:
		r = subagentStart(ctx, cfg, e, in)
	case ToolFailure:
		r = toolFailure(ctx, cfg, e, in)
	default:
		r = preToolUse(ctx, cfg, e, in)
	}
	r.session = in.SessionID
	if r.err == nil {
		r.err = ctx.Err()
	}
	return r
}

func readInput(r io.Reader) (Input, error) {
	var in Input
	if r == nil {
		return in, errors.New("no stdin")
	}
	dec := json.NewDecoder(io.LimitReader(r, maxInput))
	if err := dec.Decode(&in); err != nil {
		return in, err
	}
	return in, nil
}

func encode(o Output) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]Output{"hookSpecificOutput": o}); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

func openSeen(e env.Env, in Input) (*session.Seen, error) {
	id := in.SessionID
	if id == "" {
		return nil, nil
	}
	if in.AgentID != "" {
		id += ":" + in.AgentID
	}
	return session.Open(e.StateDir(), id)
}

func openStore(ctx context.Context, e env.Env, write bool) (*store.Store, bool, error) {
	path := e.DBPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	opt := store.Options{ReadOnly: !write, BusyTimeout: busyTimeout, Now: e.Now, Access: store.Access{Agent: e.Agent()}}
	s, err := store.Open(ctx, path, opt)
	if errors.Is(err, store.ErrSchemaOutdated) {
		opt.ReadOnly = false
		s, err = store.Open(ctx, path, opt)
	}
	if err != nil {
		return nil, false, err
	}
	return s, true, nil
}

func writeLog(e env.Env, event, sess, status, detail string, elapsed time.Duration) {
	dir := e.StateDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	path := filepath.Join(dir, LogName)
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	if sess == "" {
		sess = "-"
	}
	line := fmt.Sprintf("%s %s session=%s %s %dms", e.Now().UTC().Format(time.RFC3339), event, oneLine(sess, 128), status, elapsed.Milliseconds())
	if detail != "" {
		line += " " + oneLine(detail, maxLogDetail)
	}
	_, _ = f.WriteString(line + "\n")
	_ = f.Close()
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = string(r[:n]) + "..."
	}
	return s
}

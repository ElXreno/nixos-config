package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"mem/internal/env"
	"mem/internal/store"
)

const program = "mem"

const usage = `usage: mem [--db PATH] [--as AGENT] <command> [flags]

records:
  put KEY --kind KIND --scope S... --title T [--body TEXT | < body] [--core] [--trigger TOOL:REGEX]... [--verify CMD]
      [--ttl N|Nd|Nw|Nh, 1 to 36500 days] [--update | --supersedes OLD... | --distinct OTHER... --why TEXT | --no-gate --why TEXT]
                      write one record; a new key passes the duplicate gate first; KEY may also be --key K, flags go before or after it;
                      --body absent or - reads stdin; --update keeps kind, scopes and title when omitted
  get K... [--body] [--history]
                      show records; --body prints only the body, several keys separated by a === KEY line;
                      --history adds every previous version
  history K...        same as get K... --history
  edit K --old TEXT --new TEXT [--all] [--why TEXT]
                      replace one exact occurrence of TEXT in the body (exit 3 when it is absent or ambiguous without --all);
                      the previous version stays in history; - reads that text from stdin
  search QUERY... [--limit N] [--kind K]... [--scope S]... [--logs] [--all-status] [--all-chunks] [--no-rerank]
      [--mode fused|words|trigram | --mode vector|hybrid --embed-url URL --embed-model M [--embed-query-prefix S] [--rerank]]
                      without --mode: hybrid of vectors and words, reranked, when the embedder and the reranker are configured
                      (MEM_EMBED_URL, MEM_EMBED_MODEL, MEM_RERANK_URL); when one does not answer the fused lexical search answers
                      and stderr says so; log records only with --logs; expired state records are listed, marked EXPIRED;
                      --all-chunks lists every matching chunk of a log file and is lexical only: without --mode it selects fused,
                      with --mode vector|hybrid it is a usage error
  list [--kind K]... [--scope S]... [--status S|all] [--core]
  retire K --why TEXT
  share K... --why TEXT
                      with --as AGENT: move records of that agent into the shared memory
  topics set --map FILE.tsv [--strict] [--json]
                      topic of every record of the map (key, topic id, topic title per line, tab separated); idempotent
  topics list [--json]
                      topic id, current records, current log chunks, title
  verify K [--why TEXT]
                      the record still holds: as-of becomes now, an expired record is current again
  expire              mark state records past their ttl as expired
  retime [--since YYYY-MM-DD] [--apply]
                      give records whose latest updates changed only triggers their earlier as-of back, read from history;
                      lists them, --apply writes; such an update no longer moves the as-of
  core [--out PATH]   render core records to Markdown, budget 8000 chars of records, the fixed header is extra

hooks and tools:
  hook <event>        Claude Code command hook, JSON on stdin
  here [--cwd DIR] [--json]
                      scopes of a directory as the hooks see it: project:<name> of its git repository, host:<name>
  ui [--listen ADDR] [--allow-remote] [--state-dir D] [--relayout]
                      read-only local web dashboard of the store and the live hook turns, loopback only by default; the map layout is kept in the state dir, --relayout redoes it
  stats [--days N] [--top N] [--json]
                      recall report of the hook turns and hook.log of the last N days (default 7, 0 for all): prompt injections,
                      embed and rerank outcomes, latency, trigger fires per key, subagent hand-overs, expiring state records,
                      keys left out by the budget; --top limits the text lists (default 15), --json lists everything
  import-legacy --dir D --project P
  import-records [--scope S]... [--distinct FILE] [--conflicts FILE] [--related FILE] [--why TEXT] [--dry-run] [--json] FILE.jsonl...
                      migrated records through the put path and its duplicate gate; exit 3 lists the gate rejections
  merge --from DB [--since TIME] [--key K]... [--source S] [--why TEXT] [--dry-run] [--json]
                      current records of another database changed after --since: new keys pass the duplicate gate,
                      existing keys are updated in place, keys they superseded there are superseded here; one line per record
                      (new, update, same, conflict, failed), exit 3 on gate conflicts
  eval --transcripts GLOB --legacy-dir D --out DIR
  eval --legacy-dir D --export-texts DIR
                      queries.jsonl and docs.jsonl of the eval, the input of embed
  eval --legacy-dir D --out DIR --vectors DIR [--docs-vectors DIR] [--vec-weight F]
                      vector and fused recall, injection gate sweep
  eval failures --transcripts GLOB --turns FILE --out DIR [--since DATE] [--max N] [--horizon N] [--dry-run] [-- HOOK FLAGS...]
                      replay failed Bash calls through hook post-tool-use-failure: did its pointers name the records fetched later
  embed --url URL --model M --in DIR --out DIR [--which docs|queries|both] [--query-prefix S] [--doc-prefix S] [--docs-file F]
                      embed through POST /v1/embeddings into docs.f32, queries.f32 and meta.json
  vecdiff DIR_A DIR_B per-text cosine between two embed runs
  reindex --url URL --model M --doc-prefix S [--batch N] [--par N] [--skip-logs] [--force] [--max-docs N]
                      embed the current records and the expired state records that have no fresh vector, store the vectors in the database
  vec status [--model M --doc-prefix S]
                      counts of stored vectors, stale and missing records, hook embedding outcomes
  vec pairs [--model M] [--min-cos F] [--limit N] [--all] [--json]
                      current non-log record pairs above a cosine (--min-cos 0.5 to 1, default 0.9), by default only those
                      the lexical duplicate gate lets through
  build-en --db RU.db --en en.jsonl --out EN.db
                      English copy of a database with the FTS indexes rebuilt

exit codes: 0 ok, 1 error, 2 usage, 3 conflict (existing key, duplicate, wrong status, record of another agent)
database: --db, else $MEM_DB, else $XDG_DATA_HOME/agent-memory/mem.db; core file: $MEM_CORE, else ~/.claude/memory-core.md
agents: with $MEM_AGENT=NAME every record written gets the scope agent:NAME, that agent reads all records and changes only its own,
        and its core file and hook state live under agents/NAME; without it records of agents stay hidden, --as NAME shows those of one
embedder: $MEM_EMBED_URL, $MEM_EMBED_MODEL, $MEM_EMBED_DOC_PREFIX and $MEM_EMBED_QUERY_PREFIX stand in for the matching flags
`

type command func(e env.Env, args []string) int

var commands = map[string]command{
	"put":            cmdPut,
	"get":            cmdGet,
	"edit":           cmdEdit,
	"history":        cmdHistory,
	"search":         cmdSearch,
	"list":           cmdList,
	"retire":         cmdRetire,
	"share":          cmdShare,
	"verify":         cmdVerify,
	"expire":         cmdExpire,
	"retime":         cmdRetime,
	"core":           cmdCore,
	"hook":           cmdHook,
	"here":           cmdHere,
	"ui":             cmdUI,
	"import-legacy":  cmdImportLegacy,
	"import-records": cmdImportRecords,
	"merge":          cmdMerge,
	"eval":           cmdEval,
	"embed":          cmdEmbed,
	"vecdiff":        cmdVecdiff,
	"build-en":       cmdBuildEN,
	"reindex":        cmdReindex,
	"vec":            cmdVec,
	"topics":         cmdTopics,
	"stats":          cmdStats,
}

func Run(args []string, e env.Env) int {
	fs := flag.NewFlagSet(program, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	db := fs.String("db", e.DBFlag, "database path")
	as := fs.String("as", e.AsFlag, "also read the records of this agent; read commands and share only")
	fs.Usage = func() { _, _ = fmt.Fprint(e.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	e.DBFlag = *db
	e.AsFlag = *as
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	if rest[0] == "help" {
		_, _ = fmt.Fprint(e.Stdout, usage)
		return 0
	}
	cmd, ok := commands[rest[0]]
	if !ok {
		_, _ = fmt.Fprintf(e.Stderr, "%s: unknown command %q\n", program, rest[0])
		fs.Usage()
		return 2
	}
	return cmd(e, rest[1:])
}

type multi []string

func (m *multi) String() string { return strings.Join(*m, ",") }

func (m *multi) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*m = append(*m, p)
		}
	}
	return nil
}

type repeat []string

func (r *repeat) String() string { return strings.Join(*r, " ") }

func (r *repeat) Set(v string) error {
	*r = append(*r, v)
	return nil
}

type flags struct {
	*flag.FlagSet
	e    *env.Env
	db   *string
	json *bool

	name, synopsis, example string
}

func (f *flags) usageErr(msg string) int {
	ex := f.example
	if ex == "" {
		ex = strings.TrimSpace("mem " + f.name + " " + f.synopsis)
	}
	_, _ = fmt.Fprintf(f.e.Stderr, "mem %s: %s. example: %s\n", f.name, msg, ex)
	return 2
}

func newFlags(e *env.Env, name, synopsis string, withJSON bool) *flags {
	fs := flag.NewFlagSet(program+" "+name, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(e.Stderr, "usage: mem %s %s\n", name, synopsis)
		fs.PrintDefaults()
	}
	f := &flags{FlagSet: fs, e: e, name: name, synopsis: synopsis}
	f.db = fs.String("db", e.DBFlag, "database path")
	if withJSON {
		f.json = fs.Bool("json", false, "print JSON")
	}
	return f
}

func (f *flags) parse(args []string) ([]string, int, bool) {
	var pos []string
	for {
		out := f.Output()
		full := f.Usage
		f.Usage = func() {}
		f.SetOutput(io.Discard)
		err := f.Parse(args)
		f.SetOutput(out)
		f.Usage = full
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				f.Usage()
				return nil, 0, false
			}
			return nil, f.usageErr(err.Error()), false
		}
		consumed := len(args) - len(f.Args())
		terminated := f.endsWithTerminator(args[:consumed])
		args = f.Args()
		if terminated {
			pos = append(pos, args...)
			break
		}
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	f.e.DBFlag = *f.db
	return pos, 0, true
}

func (f *flags) endsWithTerminator(consumed []string) bool {
	for i := 0; i < len(consumed); i++ {
		a := consumed[i]
		if a == "--" {
			return i == len(consumed)-1
		}
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		fl := f.Lookup(name)
		if fl == nil {
			continue
		}
		if b, ok := fl.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
			i++
		}
	}
	return false
}

func (f *flags) set(name string) bool {
	found := false
	f.Visit(func(fl *flag.Flag) {
		if fl.Name == name {
			found = true
		}
	})
	return found
}

func (f *flags) asJSON() bool { return f.json != nil && *f.json }

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func fail(e env.Env, err error) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(e.Stderr, program+":", err)
	var (
		ee *store.ExistsError
		se *store.StatusError
		ge *store.GateError
		me *store.MatchError
		fe *store.ForeignError
	)
	switch {
	case errors.Is(err, store.ErrInvalid):
		return 2
	case errors.As(err, &ee), errors.As(err, &se), errors.As(err, &ge), errors.As(err, &me), errors.As(err, &fe):
		return 3
	}
	return 1
}

func openStore(e env.Env, readOnly bool) (*store.Store, error) {
	ctx := context.Background()
	opt := store.Options{Now: e.Now, ReadOnly: readOnly, Access: access(e)}
	if e.AsFlag != "" && e.Agent() != "" {
		return nil, fmt.Errorf("%w: --as is for the primary agent; agent %s already reads every record", store.ErrInvalid, e.Agent())
	}
	s, err := store.Open(ctx, e.DBPath(), opt)
	if readOnly && errors.Is(err, store.ErrSchemaOutdated) {
		opt.ReadOnly = false
		s, err = store.Open(ctx, e.DBPath(), opt)
	}
	if readOnly && store.IsNotExist(err) {
		return nil, fmt.Errorf("no memory database at %s; mem put creates it", e.DBPath())
	}
	return s, err
}

func access(e env.Env) store.Access {
	a := store.Access{Agent: e.Agent()}
	if e.AsFlag != "" {
		a.See = []string{e.AsFlag}
	}
	return a
}

func writeCore(e env.Env, s *store.Store) error {
	path := e.CorePath()
	if _, err := s.WriteCore(context.Background(), path); err != nil {
		return fmt.Errorf("core %s: %w", path, err)
	}
	return nil
}

func parseKinds(in []string) ([]store.Kind, error) {
	var out []store.Kind
	for _, k := range in {
		kind, err := store.ParseKind(k)
		if err != nil {
			return nil, err
		}
		out = append(out, kind)
	}
	return out, nil
}

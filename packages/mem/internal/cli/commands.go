package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"mem/internal/env"
	"mem/internal/store"
)

func cmdPut(e env.Env, args []string) int {
	f := newFlags(&e, "put", "KEY --kind KIND --scope S... --title T [--body TEXT | < body] [flags]\n--update keeps kind, scopes and title when they are omitted", true)
	f.example = "mem put KEY --kind fact --scope global --title T --body TEXT"
	key := f.String("key", "", "record key; may also be the first positional argument")
	body := f.String("body", "", "record body; absent or - reads stdin")
	kind := f.String("kind", "", "rule, fact, state, incident, howto or log")
	var scopes, supersedes, distinct multi
	var triggers repeat
	f.Var(&scopes, "scope", "global, project:<name>, host:<name> or topic:<id>; repeatable, comma-separated")
	title := f.String("title", "", "one-line title")
	core := f.Bool("core", false, "always loaded through the core file")
	f.Var(&triggers, "trigger", "TOOL:REGEX that injects this record next to a matching tool call; repeatable")
	verify := f.String("verify", "", "read-only command that verifies a state record (stored only)")
	var ttl days
	f.Var(&ttl, "ttl", "time a state record stays current without verification: days as N or Nd, weeks as Nw, hours as Nh, 1 to 36500 days (default 14 days)")
	update := f.Bool("update", false, "change the existing record in place")
	f.Var(&supersedes, "supersedes", "keys this record replaces; repeatable, comma-separated")
	f.Var(&distinct, "distinct", "flagged near-duplicates this record is distinct from; repeatable, comma-separated")
	why := f.String("why", "", "reason kept in history; required with --distinct")
	dup := f.Float64("dup", store.DefaultDup, "duplicate similarity threshold")
	rel := f.Float64("rel", store.DefaultRel, "similarity from which a neighbour is printed as check for conflict; one that shares two words of key and title is printed from 0.1")
	minShared := f.Int("min-shared", store.DefaultMinShared, "stems a duplicate must share at least")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 1 {
		return f.usageErr("unexpected arguments " + strings.Join(pos[1:], " ") + ", the body goes in --body or on stdin")
	}
	if len(pos) == 1 {
		if *key != "" && *key != pos[0] {
			return f.usageErr(fmt.Sprintf("key given twice, %q and --key %q", pos[0], *key))
		}
		*key = pos[0]
	}
	if *key == "" {
		return f.usageErr("missing KEY")
	}
	for _, s := range supersedes {
		if s == *key {
			return f.usageErr(fmt.Sprintf("--supersedes %s names the record itself; use --update to change it", s))
		}
	}
	var data []byte
	if f.set("body") && *body != "-" {
		data = []byte(*body)
	} else if *update && !f.set("body") && !stdinReady(e.Stdin, 200*time.Millisecond) {
		_, _ = fmt.Fprintln(e.Stderr, "mem put: nothing on stdin, body unchanged")
	} else {
		var err error
		if data, err = readStdin(e.Stdin); errors.Is(err, errNoStdin) {
			return f.usageErr("no body: " + err.Error())
		} else if err != nil {
			return fail(e, err)
		}
	}
	req := store.PutRequest{
		Key:        *key,
		Update:     *update,
		Supersedes: supersedes,
		Distinct:   distinct,
		Why:        *why,
		Gate:       store.Gate{Dup: *dup, Rel: *rel, MinShared: *minShared},
	}
	if *kind != "" {
		k, err := store.ParseKind(*kind)
		if err != nil {
			return fail(e, err)
		}
		req.Kind = k
	} else if !*update {
		return f.usageErr("missing --kind (new record)")
	}
	if len(scopes) > 0 {
		req.Scopes = scopes
	}
	if body := string(data); strings.TrimSpace(body) != "" || !*update {
		req.Body = &body
	}
	if f.set("title") {
		req.Title = title
	}
	if f.set("core") {
		req.Core = core
	}
	if f.set("verify") {
		req.VerifyCmd = verify
	}
	if f.set("ttl") {
		n := int(ttl)
		req.TTLDays = &n
	}
	if len(triggers) > 0 {
		ts := make([]store.Trigger, 0, len(triggers))
		for _, t := range triggers {
			tr, err := store.ParseTrigger(t)
			if err != nil {
				return fail(e, err)
			}
			ts = append(ts, tr)
		}
		req.Triggers = &ts
	}

	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	res, err := s.Put(context.Background(), req)
	var ge *store.GateError
	if errors.As(err, &ge) {
		printGate(e, req, ge)
		return 3
	}
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, res); err != nil {
			return fail(e, err)
		}
	} else {
		printPut(e, res)
	}
	if !res.Unchanged {
		if err := writeCore(e, s); err != nil {
			return fail(e, err)
		}
	}
	return 0
}

func printPut(e env.Env, res store.PutResult) {
	state := "updated"
	switch {
	case res.Created:
		state = "created"
	case res.Unchanged:
		state = "unchanged"
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\tversion %d\n", res.Key, state, res.Version)
	if res.Unchanged {
		_, _ = fmt.Fprintf(e.Stdout, "nothing changed; `mem verify %s` records that it still holds\n", res.Key)
	}
	if len(res.Superseded) > 0 {
		_, _ = fmt.Fprintf(e.Stdout, "superseded: %s\n", strings.Join(res.Superseded, ", "))
	}
	if len(res.Related) > 0 {
		_, _ = fmt.Fprintln(e.Stdout, "check for conflict:")
		for _, c := range res.Related {
			printCandidate(e.Stdout, c)
		}
	}
}

func printCandidate(w io.Writer, c store.Candidate) {
	_, _ = fmt.Fprintf(w, "  %s  similarity %.2f  [%s]  %s\n    %s\n", c.Record.Key, c.Similarity, c.Record.Kind, c.Record.Title, c.Snippet)
}

func printGate(e env.Env, req store.PutRequest, ge *store.GateError) {
	w := e.Stderr
	_, _ = fmt.Fprintf(w, "mem: %s; nothing was written\n", ge.Error())
	keys := make([]string, len(ge.Duplicates))
	for i, c := range ge.Duplicates {
		printCandidate(w, c)
		keys[i] = c.Record.Key
	}
	first := keys[0]
	_, _ = fmt.Fprintln(w, "ways forward, same body on stdin:")
	_, _ = fmt.Fprintf(w, "  same topic, new content:   mem put --key %s --update ...\n", first)
	_, _ = fmt.Fprintf(w, "  replaces it under %s:  mem put --key %s --supersedes %s ...\n", req.Key, req.Key, strings.Join(keys, ","))
	_, _ = fmt.Fprintf(w, "  genuinely different topic: mem put --key %s --distinct %s --why \"<what differs>\" ...\n", req.Key, strings.Join(keys, ","))
}

const bodyMark = "=== "

func cmdGet(e env.Env, args []string) int {
	f := newFlags(&e, "get", "KEY... [--body] [--history]", true)
	history := f.Bool("history", false, "print every previous version")
	body := f.Bool("body", false, "print only the body; several keys are separated by a `=== KEY` line")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if *body && (*history || f.asJSON()) {
		return f.usageErr("--body cannot be combined with --history or --json")
	}
	return get(e, pos, getMode{history: *history, body: *body, json: f.asJSON()})
}

func cmdHistory(e env.Env, args []string) int {
	f := newFlags(&e, "history", "KEY...", true)
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	return get(e, pos, getMode{history: true, json: f.asJSON()})
}

type getOutput struct {
	Record  store.Record    `json:"record"`
	Links   []store.Link    `json:"links,omitempty"`
	History []store.Version `json:"history,omitempty"`
}

type getMode struct {
	history, body, json bool
}

func get(e env.Env, pos []string, mode getMode) int {
	if len(pos) == 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem get: needs at least one key")
		return 2
	}
	s, err := openStore(e, true)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	outs := []getOutput{}
	code := 0
	for _, key := range pos {
		out, err := load(ctx, s, key, mode.history)
		if err != nil {
			if c := fail(e, err); len(pos) == 1 || !errors.Is(err, store.ErrNotFound) {
				return c
			}
			code = 1
			continue
		}
		outs = append(outs, out)
	}
	if mode.json {
		var v any = outs
		if len(pos) == 1 {
			v = outs[0]
		}
		if err := printJSON(e.Stdout, v); err != nil {
			return fail(e, err)
		}
		return code
	}
	for i, out := range outs {
		switch {
		case mode.body && len(pos) > 1:
			_, _ = fmt.Fprintf(e.Stdout, "%s%s\n%s\n", bodyMark, out.Record.Key, out.Record.Body)
		case mode.body:
			_, _ = fmt.Fprintf(e.Stdout, "%s\n", out.Record.Body)
		default:
			if i > 0 {
				_, _ = fmt.Fprintln(e.Stdout)
			}
			printGet(e.Stdout, out, mode.history)
		}
	}
	return code
}

func load(ctx context.Context, s *store.Store, key string, history bool) (getOutput, error) {
	r, err := s.Get(ctx, key)
	if err != nil {
		return getOutput{}, err
	}
	out := getOutput{Record: r}
	if out.Links, err = s.Links(ctx, r.Key); err != nil {
		return getOutput{}, err
	}
	if history {
		if out.History, err = s.History(ctx, r.Key); err != nil {
			return getOutput{}, err
		}
	}
	return out, nil
}

func printGet(w io.Writer, out getOutput, history bool) {
	r := out.Record
	printRecord(w, r)
	for _, l := range out.Links {
		if l.Note != "" {
			_, _ = fmt.Fprintf(w, "link: %s %s %s (%s)\n", l.Src, l.Rel, l.Dst, l.Note)
		} else {
			_, _ = fmt.Fprintf(w, "link: %s %s %s\n", l.Src, l.Rel, l.Dst)
		}
	}
	_, _ = fmt.Fprintf(w, "---\n%s\n", r.Body)
	if history {
		if len(out.History) == 0 {
			_, _ = fmt.Fprintln(w, "=== no previous versions")
		}
		for _, v := range out.History {
			_, _ = fmt.Fprintf(w, "=== version %d, replaced %s: %s\n", v.Version, v.ChangedAt.Local().Format(time.RFC3339), v.Reason)
			_, _ = fmt.Fprintf(w, "kind: %s\nstatus: %s\nscopes: %s\ncore: %v\n", v.Kind, v.Status, strings.Join(v.Scopes, ", "), v.Core)
			if v.Title != "" {
				_, _ = fmt.Fprintf(w, "title: %s\n", v.Title)
			}
			_, _ = fmt.Fprintf(w, "---\n%s\n", v.Body)
		}
	}
}

func printRecord(w io.Writer, r store.Record) {
	line := func(k, v string) {
		if v != "" {
			_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
		}
	}
	ts := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Local().Format(time.RFC3339)
	}
	line("key", r.Key)
	line("kind", string(r.Kind))
	line("status", string(r.Status))
	line("superseded_by", r.SupersededBy)
	line("scopes", strings.Join(r.Scopes, ", "))
	if r.Core {
		line("core", "true")
	}
	line("title", r.Title)
	line("version", fmt.Sprint(r.Version))
	line("valid_from", ts(r.ValidFrom))
	line("valid_to", ts(r.ValidTo))
	if r.TTLDays > 0 {
		line("ttl_days", fmt.Sprint(r.TTLDays))
	}
	line("verify", r.VerifyCmd)
	line("verified_at", ts(r.VerifiedAt))
	for _, t := range r.Triggers {
		line("trigger", t.String())
	}
	line("source", r.Source)
	line("created_at", ts(r.CreatedAt))
	line("updated_at", ts(r.UpdatedAt))
}

func cmdList(e env.Env, args []string) int {
	f := newFlags(&e, "list", "[flags]", true)
	var kinds, scopes multi
	f.Var(&kinds, "kind", "only these kinds; repeatable, comma-separated")
	f.Var(&scopes, "scope", "only records with one of these scopes; repeatable, comma-separated")
	status := f.String("status", "current", "current, superseded, expired, retired or all")
	core := f.Bool("core", false, "only core records")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem list: unexpected arguments")
		return 2
	}
	flt := store.Filter{Scopes: scopes}
	var err error
	if flt.Kinds, err = parseKinds(kinds); err != nil {
		return fail(e, err)
	}
	if *status == "all" {
		flt.AllStatus = true
	} else {
		var ss []store.Status
		for _, p := range strings.Split(*status, ",") {
			st, err := store.ParseStatus(p)
			if err != nil {
				return fail(e, err)
			}
			ss = append(ss, st)
		}
		flt.Statuses = ss
	}
	if *core {
		flt.Core = core
	}
	s, err := openStore(e, true)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	recs, err := s.List(context.Background(), flt)
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, recs))
	}
	for _, r := range recs {
		c := "-"
		if r.Core {
			c = "core"
		}
		_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Key, r.Kind, r.Status, strings.Join(r.Scopes, ","), c,
			r.UpdatedAt.Local().Format(time.DateOnly), r.Title)
	}
	return 0
}

func cmdRetire(e env.Env, args []string) int {
	f := newFlags(&e, "retire", "KEY --why TEXT", false)
	why := f.String("why", "", "reason kept in history")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		_, _ = fmt.Fprintln(e.Stderr, "mem retire: needs exactly one key")
		return 2
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	r, err := s.Retire(context.Background(), pos[0], *why)
	if err != nil {
		return fail(e, err)
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s\tretired\tversion %d\n", r.Key, r.Version)
	return fail(e, writeCore(e, s))
}

func cmdVerify(e env.Env, args []string) int {
	f := newFlags(&e, "verify", "KEY [--why TEXT]", false)
	why := f.String("why", "", "how it was checked; kept in history when an expired record comes back")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		_, _ = fmt.Fprintln(e.Stderr, "mem verify: needs exactly one key")
		return 2
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	r, err := s.Verify(context.Background(), pos[0], *why)
	if err != nil {
		return fail(e, err)
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s\tverified %s\tversion %d\n", r.Key, r.VerifiedAt.Local().Format(time.RFC3339), r.Version)
	return fail(e, writeCore(e, s))
}

func cmdRetime(e env.Env, args []string) int {
	f := newFlags(&e, "retime", "[--since YYYY-MM-DD] [--apply]", true)
	since := f.String("since", "", "only updates made on or after this local day")
	apply := f.Bool("apply", false, "write the restored dates; without it they are only listed")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem retime: unexpected arguments")
		return 2
	}
	var from time.Time
	if *since != "" {
		t, err := time.ParseInLocation(time.DateOnly, *since, time.Local)
		if err != nil {
			_, _ = fmt.Fprintln(e.Stderr, "mem retime: --since wants YYYY-MM-DD")
			return 2
		}
		from = t
	}
	s, err := openStore(e, !*apply)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	done, err := s.Retime(context.Background(), from, *apply)
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, map[string]any{"applied": *apply, "records": done}); err != nil {
			return fail(e, err)
		}
	} else {
		verb := "would restore"
		if *apply {
			verb = "restored"
		}
		for _, d := range done {
			_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\t%s -> %s\n", d.Key, verb,
				d.From.Local().Format(time.DateOnly), d.To.Local().Format(time.DateOnly))
		}
	}
	if *apply && len(done) > 0 {
		return fail(e, writeCore(e, s))
	}
	return 0
}

func cmdExpire(e env.Env, args []string) int {
	f := newFlags(&e, "expire", "", true)
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem expire: unexpected arguments")
		return 2
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	recs, err := s.Expire(context.Background())
	if err != nil {
		return fail(e, err)
	}
	keys := make([]string, len(recs))
	for i, r := range recs {
		keys[i] = r.Key
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, map[string][]string{"expired": keys}); err != nil {
			return fail(e, err)
		}
	} else {
		for _, k := range keys {
			_, _ = fmt.Fprintf(e.Stdout, "%s\texpired\n", k)
		}
	}
	if len(recs) > 0 {
		return fail(e, writeCore(e, s))
	}
	return 0
}

func cmdCore(e env.Env, args []string) int {
	f := newFlags(&e, "core", "[--out PATH]", false)
	out := f.String("out", "", "output file, - for stdout (default $MEM_CORE, else ~/.claude/memory-core.md)")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem core: unexpected arguments")
		return 2
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	if *out == "-" {
		text, err := s.RenderCore(ctx, store.CoreScopes, store.CoreBudget)
		if err != nil {
			return fail(e, err)
		}
		_, _ = fmt.Fprint(e.Stdout, text)
		return 0
	}
	path := *out
	if path == "" {
		path = e.CorePath()
	}
	n, err := s.WriteCore(ctx, path)
	if err != nil {
		return fail(e, err)
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s\t%d chars\tbudget %d\n", path, n, store.CoreBudget)
	return 0
}

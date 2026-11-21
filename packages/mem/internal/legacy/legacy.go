package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"mem/internal/env"
	"mem/internal/store"
)

const (
	DefaultLogBytes     = 10000
	DefaultJournalLines = 2
	DefaultChunkRunes   = 1200
	Why                 = "legacy"
	SourcePrefix        = "legacy:"
)

type Options struct {
	Project      string
	LogBytes     int
	JournalLines int
	ChunkRunes   int
	StateTTL     int
}

func (o Options) withDefaults() Options {
	if o.LogBytes <= 0 {
		o.LogBytes = DefaultLogBytes
	}
	if o.JournalLines <= 0 {
		o.JournalLines = DefaultJournalLines
	}
	if o.ChunkRunes <= 0 {
		o.ChunkRunes = DefaultChunkRunes
	}
	return o
}

type File struct {
	Slug    string     `json:"slug"`
	Path    string     `json:"path"`
	Layout  string     `json:"layout"`
	Kind    store.Kind `json:"kind,omitempty"`
	Index   bool       `json:"index,omitempty"`
	Log     string     `json:"log,omitempty"`
	Keys    []string   `json:"keys,omitempty"`
	Skipped string     `json:"skipped,omitempty"`
}

type Stats struct {
	Files      int                `json:"files"`
	Indexes    int                `json:"indexes"`
	Skipped    int                `json:"skipped"`
	Records    int                `json:"records"`
	LogFiles   int                `json:"log_files"`
	LogBySize  int                `json:"log_by_size"`
	LogByDates int                `json:"log_by_journal"`
	Chunks     int                `json:"chunks"`
	Links      int                `json:"links"`
	Unresolved int                `json:"unresolved"`
	ToIndex    int                `json:"to_index"`
	Kinds      map[store.Kind]int `json:"kinds"`
	Layouts    map[string]int     `json:"layouts"`
}

type Plan struct {
	Dir     string         `json:"dir"`
	Project string         `json:"project"`
	Records []store.Record `json:"-"`
	Links   []store.Link   `json:"-"`
	Files   []File         `json:"files"`
	Stats   Stats          `json:"stats"`
}

type parsed struct {
	file  File
	fm    frontmatter
	body  string
	size  int64
	mtime time.Time
}

var invalidKeyRune = regexp.MustCompile(`[^a-z0-9_.:#-]+`)

func slugKey(slug string) string {
	k := invalidKeyRune.ReplaceAllString(strings.ToLower(slug), "-")
	return strings.TrimLeft(k, "_.:#-")
}

func LogKey(slug string) string {
	return slugKey(slug) + "#1"
}

func isIndex(name, slug string) bool {
	return name == "MEMORY.md" || strings.HasSuffix(slug, "_hub") || strings.HasPrefix(slug, "archive")
}

func kindFor(slug, fmType string) store.Kind {
	switch {
	case strings.HasPrefix(slug, "feedback_"):
		return store.Rule
	case strings.HasPrefix(slug, "reference_"), strings.HasPrefix(slug, "user_"):
		return store.Fact
	case strings.HasPrefix(slug, "project_"):
		return store.State
	}
	switch fmType {
	case "feedback":
		return store.Rule
	case "project":
		return store.State
	}
	return store.Fact
}

func layoutOf(fm frontmatter, has bool) string {
	switch {
	case !has:
		return "none"
	case fm.top["type"] != "":
		return "top"
	case fm.nested["metadata"]["type"] != "":
		return "metadata"
	}
	return "partial"
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if headingRe.MatchString(line) {
			return headingText(line)
		}
		if strings.TrimSpace(line) != "" {
			return ""
		}
	}
	return ""
}

func titleFor(slug string, fm frontmatter, body string) string {
	desc := strings.Join(strings.Fields(fm.Description()), " ")
	name := strings.Join(strings.Fields(fm.Name()), " ")
	if name != "" && (normName(name) == normName(slug) || strings.Contains(strings.ToLower(desc), strings.ToLower(name))) {
		name = ""
	}
	switch {
	case name != "" && desc != "":
		return name + ": " + desc
	case desc != "":
		return desc
	case name != "":
		return name
	}
	return firstHeading(body)
}

func readDir(dir string) ([]parsed, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []parsed
	for _, ent := range ents {
		name := ent.Name()
		if ent.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		fm, body, has := splitFrontmatter(string(data))
		slug := strings.TrimSuffix(name, ".md")
		out = append(out, parsed{
			file:  File{Slug: slug, Path: path, Layout: layoutOf(fm, has), Index: isIndex(name, slug)},
			fm:    fm,
			body:  strings.TrimSpace(body),
			size:  info.Size(),
			mtime: info.ModTime().UTC().Truncate(time.Second),
		})
	}
	return out, nil
}

func Load(dir string, opt Options) (Plan, error) {
	opt = opt.withDefaults()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Plan{}, err
	}
	scope, err := store.NormalizeScope(store.ProjectScope(opt.Project))
	if err != nil || opt.Project == "" {
		return Plan{}, fmt.Errorf("%w: project %q does not form a valid scope", store.ErrInvalid, opt.Project)
	}
	files, err := readDir(abs)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Dir: abs, Project: opt.Project, Stats: Stats{Kinds: map[store.Kind]int{}, Layouts: map[string]int{}}}
	res := newResolver()
	indexes := map[string]bool{}
	for _, p := range files {
		res.add(p.file.Slug, p.fm.Name())
		indexes[p.file.Slug] = p.file.Index
	}
	dest := map[string]string{}
	for i := range files {
		p := &files[i]
		plan.Stats.Files++
		plan.Stats.Layouts[p.file.Layout]++
		if p.file.Index {
			plan.Stats.Indexes++
			continue
		}
		key := slugKey(p.file.Slug)
		if store.ValidKey(key) != nil {
			p.file.Skipped = "slug does not form a valid key"
			continue
		}
		title := titleFor(p.file.Slug, p.fm, p.body)
		if p.body == "" {
			p.body = title
		}
		if p.body == "" {
			p.file.Skipped = "empty file"
			continue
		}
		p.file.Kind = kindFor(p.file.Slug, p.fm.Type())
		switch {
		case p.size >= int64(opt.LogBytes):
			p.file.Log = "size"
		case journalLines(p.body) >= opt.JournalLines:
			p.file.Log = "journal"
		}
		if p.file.Log != "" {
			p.file.Keys = []string{key + "#1"}
		} else {
			p.file.Keys = []string{key}
		}
		dest[p.file.Slug] = p.file.Keys[0]
	}
	seenLinks := map[store.Link]bool{}
	addLinks := func(src, own, body string) {
		for _, t := range linkTargets(body) {
			l := store.Link{Src: src, Dst: t, Rel: store.Unresolved}
			if slug, ok := res.resolve(t); ok {
				if slug == own {
					continue
				}
				if d, ok := dest[slug]; ok {
					l = store.Link{Src: src, Dst: d, Rel: store.Relates}
				} else {
					l.Dst = slug
				}
			}
			if seenLinks[l] {
				continue
			}
			seenLinks[l] = true
			plan.Links = append(plan.Links, l)
			switch {
			case l.Rel == store.Relates:
				plan.Stats.Links++
			case indexes[l.Dst]:
				plan.Stats.ToIndex++
			default:
				plan.Stats.Unresolved++
			}
		}
	}
	for i := range files {
		p := &files[i]
		if p.file.Index || p.file.Skipped != "" {
			if p.file.Skipped != "" {
				plan.Stats.Skipped++
			}
			plan.Files = append(plan.Files, p.file)
			continue
		}
		key := slugKey(p.file.Slug)
		title := titleFor(p.file.Slug, p.fm, p.body)
		from := p.fm.Modified()
		if from.IsZero() {
			from = p.mtime
		}
		base := store.Record{
			Kind:      p.file.Kind,
			Scopes:    []string{scope},
			Title:     title,
			Source:    SourcePrefix + p.file.Path,
			ValidFrom: from,
			UpdatedAt: from,
		}
		if p.file.Log == "" {
			r := base
			r.Key, r.Body = key, p.body
			if r.Kind == store.State {
				r.TTLDays = opt.StateTTL
			}
			plan.Records = append(plan.Records, r)
			plan.Stats.Records++
			plan.Stats.Kinds[r.Kind]++
			addLinks(r.Key, p.file.Slug, r.Body)
			plan.Files = append(plan.Files, p.file)
			continue
		}
		plan.Stats.LogFiles++
		if p.file.Log == "size" {
			plan.Stats.LogBySize++
		} else {
			plan.Stats.LogByDates++
		}
		p.file.Keys = nil
		for n, c := range chunkBody(p.body, opt.ChunkRunes) {
			r := base
			r.Kind = store.Log
			r.Key = fmt.Sprintf("%s#%d", key, n+1)
			r.Body = c.text
			if c.heading != "" && !strings.EqualFold(c.heading, title) {
				r.Title = strings.TrimSpace(title + " / " + c.heading)
				r.Title = strings.TrimPrefix(r.Title, "/ ")
			}
			plan.Records = append(plan.Records, r)
			p.file.Keys = append(p.file.Keys, r.Key)
			plan.Stats.Chunks++
			plan.Stats.Kinds[store.Log]++
			addLinks(r.Key, p.file.Slug, r.Body)
		}
		plan.Files = append(plan.Files, p.file)
	}
	return plan, nil
}

type Result struct {
	Created   []string `json:"created,omitempty"`
	Updated   []string `json:"updated,omitempty"`
	Unchanged int      `json:"unchanged"`
	Kept      []string `json:"kept,omitempty"`
	Stale     []string `json:"stale,omitempty"`
	Retired   []string `json:"retired,omitempty"`
}

func editedOutsideImport(ctx context.Context, s *store.Store, key string) (bool, error) {
	hist, err := s.History(ctx, key)
	if err != nil || len(hist) == 0 {
		return false, err
	}
	return !strings.HasPrefix(hist[len(hist)-1].Reason, "import"), nil
}

func sameAsStored(old, r store.Record) bool {
	return old.Kind == r.Kind && slices.Equal(old.Scopes, r.Scopes) && !old.Core && old.Title == r.Title &&
		old.Body == r.Body && old.Source == r.Source && old.VerifyCmd == "" && len(old.Triggers) == 0
}

func Apply(ctx context.Context, s *store.Store, plan Plan, retireStale bool) (Result, error) {
	var res Result
	var recs []store.Record
	for _, r := range plan.Records {
		old, err := s.Get(ctx, r.Key)
		switch {
		case errors.Is(err, store.ErrNotFound):
			recs = append(recs, r)
			continue
		case err != nil:
			return res, err
		}
		if sameAsStored(old, r) {
			recs = append(recs, r)
			continue
		}
		edited, err := editedOutsideImport(ctx, s, r.Key)
		if err != nil {
			return res, err
		}
		if edited {
			res.Kept = append(res.Kept, r.Key)
			continue
		}
		recs = append(recs, r)
	}
	up, err := s.Upsert(ctx, recs, plan.Links, Why)
	if err != nil {
		return res, err
	}
	res.Created, res.Updated, res.Unchanged = up.Created, up.Updated, up.Unchanged
	produced := map[string]bool{}
	for _, r := range plan.Records {
		produced[r.Key] = true
	}
	current, err := s.List(ctx, store.Filter{})
	if err != nil {
		return res, err
	}
	prefix := SourcePrefix + plan.Dir + string(filepath.Separator)
	for _, r := range current {
		if strings.HasPrefix(r.Source, prefix) && !produced[r.Key] {
			res.Stale = append(res.Stale, r.Key)
		}
	}
	if retireStale {
		for _, k := range res.Stale {
			if _, err := s.Retire(ctx, k, "legacy import no longer produces this record"); err != nil {
				return res, err
			}
			res.Retired = append(res.Retired, k)
		}
	}
	if len(res.Created)+len(res.Updated)+len(res.Retired) > 0 {
		if err := s.Optimize(ctx); err != nil {
			return res, err
		}
	}
	return res, nil
}

func Import(ctx context.Context, s *store.Store, dir string, opt Options, retireStale bool) (Plan, Result, error) {
	plan, err := Load(dir, opt)
	if err != nil {
		return plan, Result{}, err
	}
	res, err := Apply(ctx, s, plan, retireStale)
	return plan, res, err
}

const usage = "usage: mem import-legacy --dir D --project P [--db PATH] [--dry-run] [--retire-stale] [--json]\n" +
	"       [--log-bytes N] [--journal-lines N] [--chunk N] [--state-ttl DAYS]\n"

func Run(args []string, e env.Env) int {
	fs := flag.NewFlagSet("mem import-legacy", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(e.Stderr, usage)
		fs.PrintDefaults()
	}
	dir := fs.String("dir", "", "legacy memory directory, read only")
	project := fs.String("project", "", "project name for the project:<name> scope")
	db := fs.String("db", e.DBFlag, "database path")
	dry := fs.Bool("dry-run", false, "parse and report, write nothing")
	retire := fs.Bool("retire-stale", false, "retire current records from this directory that the import no longer produces")
	asJSON := fs.Bool("json", false, "print JSON")
	logBytes := fs.Int("log-bytes", DefaultLogBytes, "files of at least this many bytes become log chunks")
	journal := fs.Int("journal-lines", DefaultJournalLines, "files with at least this many date-led lines become log chunks")
	chunkRunes := fs.Int("chunk", DefaultChunkRunes, "target log chunk size in characters")
	stateTTL := fs.Int("state-ttl", 0, "ttl_days of imported state records, 0 keeps the store default of 14; the ttl counts from the file date")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem import-legacy: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *dir == "" || *project == "" {
		_, _ = fmt.Fprintln(e.Stderr, "mem import-legacy: --dir and --project are required")
		return 2
	}
	e.DBFlag = *db
	if *stateTTL < 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem import-legacy: --state-ttl must not be negative")
		return 2
	}
	opt := Options{Project: *project, LogBytes: *logBytes, JournalLines: *journal, ChunkRunes: *chunkRunes, StateTTL: *stateTTL}
	plan, err := Load(*dir, opt)
	if err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "mem import-legacy:", err)
		if errors.Is(err, store.ErrInvalid) {
			return 2
		}
		return 1
	}
	out := struct {
		Plan    Plan     `json:"plan"`
		PastTTL []string `json:"state_past_ttl,omitempty"`
		Result  *Result  `json:"result,omitempty"`
		DB      string   `json:"db,omitempty"`
	}{Plan: plan, PastTTL: PastTTL(plan, e.Now())}
	if !*dry {
		ctx := context.Background()
		s, err := store.Open(ctx, e.DBPath(), store.Options{Now: e.Now})
		if err != nil {
			_, _ = fmt.Fprintln(e.Stderr, "mem import-legacy:", err)
			return 1
		}
		defer func() { _ = s.Close() }()
		res, err := Apply(ctx, s, plan, *retire)
		if err != nil {
			_, _ = fmt.Fprintln(e.Stderr, "mem import-legacy:", err)
			return 1
		}
		out.Result, out.DB = &res, s.Path()
	}
	if *asJSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(out); err != nil {
			return 1
		}
		return 0
	}
	report(e, plan, out.PastTTL, out.Result, out.DB)
	return 0
}

func PastTTL(plan Plan, now time.Time) []string {
	var out []string
	for _, r := range plan.Records {
		if r.Kind != store.State {
			continue
		}
		ttl := r.TTLDays
		if ttl <= 0 {
			ttl = store.DefaultStateTTL
		}
		if !now.Before(r.UpdatedAt.Add(time.Duration(ttl) * 24 * time.Hour)) {
			out = append(out, r.Key)
		}
	}
	return out
}

func report(e env.Env, plan Plan, pastTTL []string, res *Result, db string) {
	st := plan.Stats
	w := e.Stdout
	_, _ = fmt.Fprintf(w, "dir %s, scope %s\n", plan.Dir, store.ProjectScope(plan.Project))
	_, _ = fmt.Fprintf(w, "files %d: %d indexes skipped, %d unusable, %d records, %d log files (%d by size, %d by journal lines) in %d chunks\n",
		st.Files, st.Indexes, st.Skipped, st.Records, st.LogFiles, st.LogBySize, st.LogByDates, st.Chunks)
	var kinds []string
	for _, k := range store.Kinds {
		if n := st.Kinds[k]; n > 0 {
			kinds = append(kinds, fmt.Sprintf("%s %d", k, n))
		}
	}
	_, _ = fmt.Fprintf(w, "kinds: %s\n", strings.Join(kinds, ", "))
	var layouts []string
	for _, l := range []string{"top", "metadata", "partial", "none"} {
		if n := st.Layouts[l]; n > 0 {
			layouts = append(layouts, fmt.Sprintf("%s %d", l, n))
		}
	}
	_, _ = fmt.Fprintf(w, "frontmatter: %s\n", strings.Join(layouts, ", "))
	_, _ = fmt.Fprintf(w, "links: %d resolved, %d to index files, %d unresolved\n", st.Links, st.ToIndex, st.Unresolved)
	if n := st.Kinds[store.State]; n > 0 {
		_, _ = fmt.Fprintf(w, "state: %d records, %d dated past their ttl; the next `mem expire` or session start marks those expired (--state-ttl sets a longer ttl)\n", n, len(pastTTL))
	}
	for _, f := range plan.Files {
		if f.Skipped != "" {
			_, _ = fmt.Fprintf(w, "skipped %s: %s\n", f.Slug, f.Skipped)
		}
	}
	if res == nil {
		_, _ = fmt.Fprintln(w, "dry run, nothing written")
		return
	}
	_, _ = fmt.Fprintf(w, "db %s: %d created, %d updated, %d unchanged, %d kept (edited after import), %d stale, %d retired\n",
		db, len(res.Created), len(res.Updated), res.Unchanged, len(res.Kept), len(res.Stale), len(res.Retired))
	for _, k := range res.Kept {
		_, _ = fmt.Fprintf(w, "kept %s\n", k)
	}
	if len(res.Stale) > len(res.Retired) {
		for _, k := range res.Stale {
			_, _ = fmt.Fprintf(w, "stale %s\n", k)
		}
		_, _ = fmt.Fprintln(w, "rerun with --retire-stale to retire stale records")
	}
}

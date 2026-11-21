package eval

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"
)

type AccessKind uint8

const (
	AccessRead AccessKind = iota
	AccessBashRead
	AccessWrite
	AccessScript
)

type Access struct {
	At      time.Time
	Spawn   time.Time
	Session string
	Slug    string
	Kind    AccessKind
	Tool    string
	Sub     bool
	Index   bool
}

const (
	DatesDB     = "db"
	DatesEvents = "events"
	DatesBoth   = "both"
)

type GoldOptions struct {
	Tier    int
	Scripts bool
	Direct  bool
	Bash    bool
	Sub     bool
	Dates   string
	Import  time.Time
}

func DefaultGold() GoldOptions {
	return GoldOptions{Direct: true, Dates: DatesDB}
}

func (g GoldOptions) Sources() string {
	var parts []string
	if g.Direct {
		parts = append(parts, "direct")
	}
	if g.Bash {
		parts = append(parts, "bash")
	}
	if g.Sub {
		parts = append(parts, "subagent")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}

func (g GoldOptions) TierName() string {
	name := fmt.Sprintf("tier%d", g.Tier)
	if g.Tier >= 2 {
		name = "tier2-" + g.Dates
	}
	if g.Scripts && g.Tier >= 1 {
		name += "+scripts"
	}
	return name
}

func (g GoldOptions) Validate() error {
	if g.Tier < 0 || g.Tier > 2 {
		return fmt.Errorf("--gold-tier must be 0, 1 or 2")
	}
	if g.Dates != DatesDB && g.Dates != DatesEvents && g.Dates != DatesBoth {
		return fmt.Errorf("--gold-dates must be db, events or both")
	}
	if !g.Direct && !g.Bash && !g.Sub {
		return fmt.Errorf("at least one gold source must be enabled")
	}
	return nil
}

type goldEnv struct {
	dbDate map[string]time.Time
	events map[string][]time.Time
	until  time.Time
}

func newGoldEnv(dbDate map[string]time.Time, writes []Access, until time.Time, scripts bool) *goldEnv {
	env := &goldEnv{dbDate: dbDate, events: map[string][]time.Time{}, until: until}
	for _, w := range writes {
		if w.At.IsZero() || w.Kind != AccessWrite && (!scripts || w.Kind != AccessScript) {
			continue
		}
		if !until.IsZero() && w.At.After(until) {
			continue
		}
		env.events[w.Slug] = append(env.events[w.Slug], w.At)
	}
	for _, ts := range env.events {
		slices.SortFunc(ts, func(a, b time.Time) int { return a.Compare(b) })
	}
	return env
}

func (e *goldEnv) writtenAfter(slug string, start time.Time, mode string) bool {
	if start.IsZero() {
		return false
	}
	if mode != DatesEvents {
		if d := e.dbDate[slug]; !d.IsZero() && d.Add(time.Second).After(start) {
			return true
		}
	}
	if mode != DatesDB {
		ts := e.events[slug]
		i := sort.Search(len(ts), func(i int) bool { return !ts[i].Before(start) })
		if i < len(ts) {
			return true
		}
	}
	return false
}

func (g GoldOptions) candidates(p Prompt) []string {
	var out []string
	if g.Direct {
		for _, s := range p.Gold {
			out = addSlug(out, s)
		}
	}
	if g.Bash {
		for _, s := range p.BashGold {
			out = addSlug(out, s)
		}
	}
	if g.Sub {
		for _, s := range p.SubGold {
			out = addSlug(out, s)
		}
	}
	return out
}

type goldPick struct {
	gold         []string
	candidates   int
	sameTurn     int
	laterWritten int
}

func (g GoldOptions) pick(p Prompt, env *goldEnv) goldPick {
	cands := g.candidates(p)
	res := goldPick{candidates: len(cands)}
	for _, s := range cands {
		if g.Tier >= 1 && (slices.Contains(p.WrittenAll, s) || g.Scripts && slices.Contains(p.ScriptWrites, s)) {
			res.sameTurn++
			continue
		}
		if g.Tier >= 2 && env != nil && env.writtenAfter(s, p.Start, g.Dates) {
			res.laterWritten++
			continue
		}
		res.gold = append(res.gold, s)
	}
	return res
}

type GoldCounts struct {
	BashCalls        int `json:"bash_calls_naming_memory"`
	BashOther        int `json:"bash_calls_other_memory_use"`
	BashWithEffect   int `json:"bash_calls_with_parsed_effect"`
	BashReads        int `json:"bash_reads"`
	BashIndexReads   int `json:"bash_index_reads"`
	BashWrites       int `json:"bash_writes"`
	BashNoEffect     int `json:"bash_calls_unclassified"`
	BashScript       int `json:"bash_calls_unclassified_with_interpreter"`
	BashScriptWrites int `json:"bash_script_writes"`
	BashOrphan       int `json:"bash_reads_outside_genuine_turns"`
	SubReads         int `json:"subagent_reads"`
	SubBashReads     int `json:"subagent_bash_reads"`
	SubIndexReads    int `json:"subagent_index_reads"`
	SubWrites        int `json:"subagent_writes"`
	SubUnattributed  int `json:"subagent_accesses_unattributed"`
	SubCrossTurn     int `json:"subagent_accesses_in_a_later_turn_than_the_spawn"`
	SubFiles         int `json:"subagent_files_scanned"`
}

func (c *GoldCounts) add(o GoldCounts) {
	c.BashCalls += o.BashCalls
	c.BashOther += o.BashOther
	c.BashWithEffect += o.BashWithEffect
	c.BashReads += o.BashReads
	c.BashIndexReads += o.BashIndexReads
	c.BashWrites += o.BashWrites
	c.BashNoEffect += o.BashNoEffect
	c.BashScript += o.BashScript
	c.BashScriptWrites += o.BashScriptWrites
	c.BashOrphan += o.BashOrphan
	c.SubReads += o.SubReads
	c.SubBashReads += o.SubBashReads
	c.SubIndexReads += o.SubIndexReads
	c.SubWrites += o.SubWrites
	c.SubUnattributed += o.SubUnattributed
	c.SubCrossTurn += o.SubCrossTurn
	c.SubFiles += o.SubFiles
}

type ToolCount struct {
	Tool  string `json:"tool"`
	Main  int    `json:"main"`
	Sub   int    `json:"subagent"`
	Index int    `json:"of_which_index"`
	Files int    `json:"distinct_files"`
	Dated int    `json:"with_timestamp"`
}

type WriteStats struct {
	Events     int         `json:"events"`
	Files      int         `json:"distinct_files"`
	FilesNoIdx int         `json:"distinct_non_index_files"`
	Undated    int         `json:"undated"`
	Tools      []ToolCount `json:"by_tool"`
	BashByOp   []ToolCount `json:"bash_by_op"`
}

func writeStats(writes []Access) WriteStats {
	var st WriteStats
	st.Events = len(writes)
	files := map[string]bool{}
	filesNoIdx := map[string]bool{}
	byTool := map[string]*ToolCount{}
	byOp := map[string]*ToolCount{}
	toolFiles := map[string]map[string]bool{}
	opFiles := map[string]map[string]bool{}
	bump := func(m map[string]*ToolCount, fm map[string]map[string]bool, name string, w Access) {
		tc := m[name]
		if tc == nil {
			tc = &ToolCount{Tool: name}
			m[name] = tc
			fm[name] = map[string]bool{}
		}
		if w.Sub {
			tc.Sub++
		} else {
			tc.Main++
		}
		if w.Index {
			tc.Index++
		}
		if !w.At.IsZero() {
			tc.Dated++
		}
		fm[name][w.Slug] = true
	}
	for _, w := range writes {
		files[w.Slug] = true
		if !w.Index {
			filesNoIdx[w.Slug] = true
		}
		if w.At.IsZero() {
			st.Undated++
		}
		tool := w.Tool
		if op, ok := strings.CutPrefix(w.Tool, "Bash:"); ok {
			bump(byOp, opFiles, op, w)
			tool = "Bash"
		}
		bump(byTool, toolFiles, tool, w)
	}
	collect := func(m map[string]*ToolCount, fm map[string]map[string]bool) []ToolCount {
		var out []ToolCount
		for n, tc := range m {
			tc.Files = len(fm[n])
			out = append(out, *tc)
		}
		slices.SortFunc(out, func(a, b ToolCount) int {
			if d := (b.Main + b.Sub) - (a.Main + a.Sub); d != 0 {
				return d
			}
			return strings.Compare(a.Tool, b.Tool)
		})
		return out
	}
	st.Files, st.FilesNoIdx = len(files), len(filesNoIdx)
	st.Tools, st.BashByOp = collect(byTool, toolFiles), collect(byOp, opFiles)
	return st
}

type MatrixRow struct {
	Tier             string  `json:"tier"`
	Sources          string  `json:"sources"`
	Prompts          int     `json:"gold_prompts"`
	Sessions         int     `json:"sessions"`
	GoldPairs        int     `json:"gold_files"`
	GoldFiles        int     `json:"distinct_gold_files"`
	LogPairs         int     `json:"gold_files_log_only"`
	LogShare         float64 `json:"log_only_share"`
	LogOnlyPrompts   int     `json:"prompts_log_only"`
	MixedPrompts     int     `json:"prompts_mixed"`
	RegularPrompts   int     `json:"prompts_regular_only"`
	InTier0          int     `json:"prompts_in_tier0"`
	NewVsTier0       int     `json:"prompts_not_in_tier0"`
	PairsInTier0     int     `json:"files_in_tier0"`
	DroppedSameTurn  int     `json:"files_dropped_same_turn"`
	DroppedLater     int     `json:"files_dropped_written_later"`
	DroppedPrompts   int     `json:"prompts_emptied_by_tier"`
	TouchedNotGold   int     `json:"local_prompts_with_memory_access_not_gold"`
	Negatives        int     `json:"negatives"`
	MissingFromDB    int     `json:"candidate_files_missing_from_db"`
	UndatedAfterTier int     `json:"prompts_without_timestamp"`
}

type GoldReport struct {
	Tier       string      `json:"tier"`
	Sources    string      `json:"sources"`
	Dates      string      `json:"dates"`
	Import     string      `json:"import_time,omitempty"`
	Selected   MatrixRow   `json:"selected"`
	Counts     GoldCounts  `json:"counts"`
	Writes     WriteStats  `json:"write_events"`
	DatedSlugs int         `json:"known_slugs_with_db_date"`
	KnownSlugs int         `json:"known_slugs"`
	Matrix     []MatrixRow `json:"matrix,omitempty"`
}

func goldReport(cfg Config, g GoldOptions, env *goldEnv, parsed parsedAll, perFile [][]Prompt, local []Prompt, known, logSlug map[string]bool, negatives int) (*GoldReport, error) {
	var all []Prompt
	for _, ps := range perFile {
		all = append(all, ps...)
	}
	rep := &GoldReport{Tier: g.TierName(), Sources: g.Sources(), Dates: g.Dates, Counts: parsed.gold, Writes: writeStats(parsed.writes), KnownSlugs: len(known)}
	if !g.Import.IsZero() {
		rep.Import = g.Import.UTC().Format(time.RFC3339)
	}
	for s := range known {
		if !env.dbDate[s].IsZero() {
			rep.DatedSlugs++
		}
	}
	tier0 := tier0Gold(all, known)
	rep.Selected = matrixRow(g, all, local, env, known, logSlug, negatives, tier0)
	if cfg.GoldMatrix != "" {
		rep.Matrix = buildMatrix(g, all, local, env, known, logSlug, negatives)
		if err := writeFile(cfg.GoldMatrix, func(w *bufio.Writer) error { return writeMatrix(w, rep.Matrix) }); err != nil {
			return nil, err
		}
	}
	if cfg.WriteEvents != "" {
		if err := writeFile(cfg.WriteEvents, func(w *bufio.Writer) error { return writeWriteEvents(w, parsed.writes) }); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

func touched(p Prompt) bool {
	return len(p.Gold) > 0 || p.IndexReads > 0 || p.MemoryUse || p.TailTouched || len(p.BashGold) > 0 || len(p.SubGold) > 0 || len(p.WrittenAll) > 0 || len(p.ScriptWrites) > 0
}

func matrixRow(g GoldOptions, all, local []Prompt, env *goldEnv, known, logSlug map[string]bool, negatives int, tier0 map[string]map[string]bool) MatrixRow {
	row := MatrixRow{Tier: g.TierName(), Sources: g.Sources(), Negatives: negatives}
	sessions := map[string]bool{}
	files := map[string]bool{}
	goldSet := map[string]bool{}
	for _, p := range all {
		pk := g.pick(p, env)
		var gold []string
		for _, s := range pk.gold {
			if known[s] {
				gold = append(gold, s)
			}
		}
		for _, s := range g.candidates(p) {
			if !known[s] {
				row.MissingFromDB++
			}
		}
		row.DroppedSameTurn += pk.sameTurn
		row.DroppedLater += pk.laterWritten
		if pk.candidates > 0 && len(pk.gold) == 0 {
			row.DroppedPrompts++
		}
		if len(gold) == 0 {
			continue
		}
		id := fmt.Sprintf("%s:%d", p.Session, p.Turn)
		goldSet[id] = true
		row.Prompts++
		sessions[p.Session] = true
		if p.Start.IsZero() && g.Tier >= 2 {
			row.UndatedAfterTier++
		}
		logs := 0
		for _, s := range gold {
			row.GoldPairs++
			files[s] = true
			if logSlug[s] {
				row.LogPairs++
				logs++
			}
			if tier0[id][s] {
				row.PairsInTier0++
			}
		}
		switch {
		case logs == len(gold):
			row.LogOnlyPrompts++
		case logs == 0:
			row.RegularPrompts++
		default:
			row.MixedPrompts++
		}
		if _, ok := tier0[id]; ok {
			row.InTier0++
		}
	}
	row.NewVsTier0 = row.Prompts - row.InTier0
	row.Sessions, row.GoldFiles = len(sessions), len(files)
	if row.GoldPairs > 0 {
		row.LogShare = float64(row.LogPairs) / float64(row.GoldPairs)
	}
	for _, p := range local {
		if touched(p) && !goldSet[fmt.Sprintf("%s:%d", p.Session, p.Turn)] {
			row.TouchedNotGold++
		}
	}
	return row
}

func tier0Gold(all []Prompt, known map[string]bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, p := range all {
		var gold []string
		for _, s := range p.Gold {
			if known[s] {
				gold = append(gold, s)
			}
		}
		if len(gold) == 0 {
			continue
		}
		set := map[string]bool{}
		for _, s := range gold {
			set[s] = true
		}
		out[fmt.Sprintf("%s:%d", p.Session, p.Turn)] = set
	}
	return out
}

var matrixSources = [][3]bool{
	{true, false, false}, {false, true, false}, {false, false, true},
	{true, true, false}, {true, false, true}, {false, true, true}, {true, true, true},
}

func buildMatrix(base GoldOptions, all, local []Prompt, env *goldEnv, known, logSlug map[string]bool, negatives int) []MatrixRow {
	tier0 := tier0Gold(all, known)
	var rows []MatrixRow
	tiers := []GoldOptions{{Tier: 0}, {Tier: 1}, {Tier: 2, Dates: DatesDB}, {Tier: 2, Dates: DatesEvents}, {Tier: 2, Dates: DatesBoth}, {Tier: 1, Scripts: true}, {Tier: 2, Dates: DatesBoth, Scripts: true}}
	for _, t := range tiers {
		for _, s := range matrixSources {
			g := t
			g.Direct, g.Bash, g.Sub = s[0], s[1], s[2]
			if g.Dates == "" {
				g.Dates = base.Dates
			}
			rows = append(rows, matrixRow(g, all, local, env, known, logSlug, negatives, tier0))
		}
	}
	return rows
}

func writeMatrix(w io.Writer, rows []MatrixRow) error {
	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintln(bw, "tier\tsources\tgold_prompts\tsessions\tgold_files\tdistinct_files\tlog_only_files\tlog_share\tprompts_log_only\tprompts_mixed\tprompts_regular\tin_tier0\tnot_in_tier0\tfiles_in_tier0\tdropped_same_turn\tdropped_written_later\tprompts_emptied\ttouched_not_gold\tnegatives\tmissing_from_db"); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(bw, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%.4f\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", r.Tier, r.Sources, r.Prompts, r.Sessions, r.GoldPairs,
			r.GoldFiles, r.LogPairs, r.LogShare, r.LogOnlyPrompts, r.MixedPrompts, r.RegularPrompts, r.InTier0, r.NewVsTier0, r.PairsInTier0, r.DroppedSameTurn,
			r.DroppedLater, r.DroppedPrompts, r.TouchedNotGold, r.Negatives, r.MissingFromDB); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func writeWriteEvents(w io.Writer, writes []Access) error {
	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintln(bw, "session\ttime\tslug\ttool\tvia\tindex"); err != nil {
		return err
	}
	sorted := slices.Clone(writes)
	slices.SortStableFunc(sorted, func(a, b Access) int {
		if d := a.At.Compare(b.At); d != 0 {
			return d
		}
		return strings.Compare(a.Slug, b.Slug)
	})
	for _, a := range sorted {
		via := "main"
		if a.Sub {
			via = "subagent"
		}
		ts := ""
		if !a.At.IsZero() {
			ts = a.At.UTC().Format(time.RFC3339Nano)
		}
		if _, err := fmt.Fprintf(bw, "%s\t%s\t%s\t%s\t%s\t%s\n", a.Session, ts, a.Slug, a.Tool, via, boolCol(a.Index)); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func printGold(w io.Writer, g *GoldReport) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	s := g.Selected
	p("gold %s, sources %s, dates %s: %d prompts in %d sessions, %d files (%d distinct), %.1f%% of files stored as log, %d of the prompts in tier0 gold, %d new; %d files dropped for a write in the same turn, %d for a later write\n",
		g.Tier, g.Sources, g.Dates, s.Prompts, s.Sessions, s.GoldPairs, s.GoldFiles, 100*s.LogShare, s.InTier0, s.NewVsTier0, s.DroppedSameTurn, s.DroppedLater)
	w1 := g.Writes
	p("memory writes found: %d events on %d files (%d outside the indexes)", w1.Events, w1.Files, w1.FilesNoIdx)
	for _, t := range w1.Tools {
		p("; %s %d main + %d subagent", t.Tool, t.Main, t.Sub)
	}
	p("\n")
	c := g.Counts
	p("bash: %d calls name this memory directory, %d with a parsed read or write (%d reads, %d writes), %d unclassified (%d of them run an interpreter), %d other memory use (other projects, mem CLI); subagent reads %d, bash reads %d, writes %d, %d unattributed\n",
		c.BashCalls, c.BashWithEffect, c.BashReads, c.BashWrites, c.BashNoEffect, c.BashScript, c.BashOther, c.SubReads, c.SubBashReads, c.SubWrites, c.SubUnattributed)
	if len(g.Matrix) == 0 {
		return
	}
	p("\n%-13s %-20s %6s %5s %6s %7s %6s %6s %6s %6s %6s %6s %6s %6s\n", "tier", "sources", "prompt", "sess", "files", "logshr", "logpr", "mixed", "reg", "in0", "new0", "dsame", "dlater", "empty")
	for _, r := range g.Matrix {
		p("%-13s %-20s %6d %5d %6d %7.3f %6d %6d %6d %6d %6d %6d %6d %6d\n", r.Tier, r.Sources, r.Prompts, r.Sessions, r.GoldPairs, r.LogShare, r.LogOnlyPrompts,
			r.MixedPrompts, r.RegularPrompts, r.InTier0, r.NewVsTier0, r.DroppedSameTurn, r.DroppedLater, r.DroppedPrompts)
	}
}

func (g GoldOptions) normalized() GoldOptions {
	if !g.Direct && !g.Bash && !g.Sub {
		g.Direct = true
	}
	if g.Dates == "" {
		g.Dates = DatesDB
	}
	return g
}

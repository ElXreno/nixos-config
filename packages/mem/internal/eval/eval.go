package eval

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"mem/internal/env"
	"mem/internal/legacy"
	"mem/internal/recall"
	"mem/internal/store"
)

const (
	DefaultGlob      = "~/.claude/projects/*/*.jsonl"
	DefaultProject   = "legacy"
	DefaultLimit     = 8
	DefaultDepth     = 20
	DefaultTail      = 600
	DefaultMinChars  = 40
	DefaultMaxQuery  = 8000
	DefaultNegatives = 0
	DefaultSeed      = 1
	PromptPrefix     = 60
	DBName           = "eval.db"
)

type Config struct {
	Glob       string
	LegacyDir  string
	Out        string
	Project    string
	DB         string
	Limit      int
	Depth      int
	Tail       int
	MinChars   int
	MaxQuery   int
	Workers    int
	Recall     recall.Settings
	AllPrompts bool
	Negatives  int
	Seed       uint64
	Cases      string
	Sweep      string
	Until      time.Time

	ExportTexts     string
	Vectors         string
	DocsVectors     string
	VecWeight       float64
	VecDepth        int
	LexicalRef      bool
	AllowOtherTexts bool

	LogsGate   bool
	Folds      int
	FIRBudgets []float64
	Pools      []string
	LexRegular bool
	LexSweep   bool

	Gold              GoldOptions
	GoldMatrix        string
	WriteEvents       string
	AllowExtraVectors bool
}

type Variant struct {
	Mode   store.Mode
	Policy string
}

func (r Variant) Name() string {
	if r.Policy == "hook" {
		return "hook"
	}
	return r.Mode.String() + "/" + r.Policy
}

var Runs = []Variant{
	{store.Fused, "all"}, {store.WordsOnly, "all"}, {store.TrigramOnly, "all"},
	{store.Fused, "nolog"}, {store.WordsOnly, "nolog"}, {store.TrigramOnly, "nolog"},
	{store.Fused, "hook"},
}

func runIndex(mode store.Mode, policy string) int {
	return slices.Index(Runs, Variant{mode, policy})
}

type Metrics struct {
	Run     string  `json:"run"`
	Gold    string  `json:"gold"`
	Prompts int     `json:"prompts"`
	Recall4 float64 `json:"recall_at_4"`
	Recall8 float64 `json:"recall_at_8"`
	Hit4    float64 `json:"hit_at_4"`
	Hit8    float64 `json:"hit_at_8"`
	MRR     float64 `json:"mrr"`
}

type Quantiles struct {
	N   int     `json:"n"`
	P10 float64 `json:"p10"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
}

type Calibration struct {
	Run             string    `json:"run"`
	GoldOverlap     Quantiles `json:"gold_overlap"`
	OtherOverlap    Quantiles `json:"other_overlap"`
	GoldWordsBM25   Quantiles `json:"gold_words_bm25"`
	OtherWordsBM25  Quantiles `json:"other_words_bm25"`
	TopGoldOverlap  Quantiles `json:"top1_gold_overlap"`
	TopOtherOverlap Quantiles `json:"top1_other_overlap"`
}

type Summary struct {
	Transcripts   int             `json:"transcripts"`
	Counts        Counts          `json:"counts"`
	DB            string          `json:"db"`
	Records       int             `json:"records"`
	LogRecords    int             `json:"log_records"`
	Evaluated     int             `json:"prompts_with_gold"`
	ShortTail     int             `json:"prompts_with_assistant_tail"`
	GoldFiles     int             `json:"gold_files"`
	GoldMissing   int             `json:"gold_files_missing_from_db"`
	GoldLog       int             `json:"gold_files_stored_as_log"`
	GoldWritten   int             `json:"gold_files_written_in_turn"`
	ReadOnlyGold  int             `json:"prompts_with_unwritten_gold"`
	Depth         int             `json:"depth"`
	Metrics       []Metrics       `json:"metrics"`
	Calibration   []Calibration   `json:"calibration"`
	Settings      recall.Settings `json:"hook_settings"`
	NegativePool  int             `json:"negative_pool"`
	Seed          uint64          `json:"negative_seed"`
	Until         string          `json:"until,omitempty"`
	Hook          []HookStats     `json:"hook"`
	Cases         []CaseResult    `json:"cases,omitempty"`
	SweepRows     int             `json:"sweep_rows,omitempty"`
	SweepKnee     *SweepRow       `json:"sweep_knee,omitempty"`
	CrossFit      []CrossFit      `json:"crossfit,omitempty"`
	Gold          *GoldReport     `json:"gold,omitempty"`
	SearchSeconds float64         `json:"search_seconds"`
	ParseSeconds  float64         `json:"parse_seconds"`
}

type query struct {
	Prompt
	caseName string
	gold     []string
	missing  int
	ranks    [][]rankedHit
	cands    []recall.Candidate
	inj      recall.Injection
}

type rankedHit struct {
	slug string
	hit  store.Hit
}

func expandGlob(glob, home string) ([]string, error) {
	files, err := filepath.Glob(expandHome(glob, home))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		if strings.Contains(f, string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(f); err == nil && info.Mode().IsRegular() {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out, nil
}

func legacyDirs(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	dirs := []string{abs}
	if real, err := filepath.EvalSymlinks(abs); err == nil && real != abs {
		dirs = append(dirs, real)
	}
	return dirs, nil
}

func sessionOf(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

func PromptHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

func promptPrefix(text string) string {
	flat := strings.Join(strings.Fields(text), " ")
	r := []rune(flat)
	if len(r) > PromptPrefix {
		r = r[:PromptPrefix]
	}
	return string(r)
}

func openDB(ctx context.Context, e env.Env, cfg Config) (*store.Store, *legacy.Result, error) {
	if cfg.DB != "" {
		s, err := store.Open(ctx, cfg.DB, store.Options{ReadOnly: true, Now: e.Now})
		return s, nil, err
	}
	path := filepath.Join(cfg.Out, DBName)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
	}
	s, err := store.Open(ctx, path, store.Options{Now: e.Now})
	if err != nil {
		return nil, nil, err
	}
	_, res, err := legacy.Import(ctx, s, cfg.LegacyDir, legacy.Options{Project: cfg.Project}, false)
	if err != nil {
		_ = s.Close()
		return nil, nil, err
	}
	return s, &res, nil
}

type slugInfo struct {
	keySlug map[string]string
	logSlug map[string]bool
	dates   map[string]time.Time
	records int
	logs    int
}

func slugMap(ctx context.Context, s *store.Store) (slugInfo, error) {
	recs, err := s.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return slugInfo{}, err
	}
	keySlug := map[string]string{}
	logSlug := map[string]bool{}
	dates := map[string]time.Time{}
	logs := 0
	for _, r := range recs {
		if r.Kind == store.Log {
			logs++
		}
		path, ok := strings.CutPrefix(r.Source, legacy.SourcePrefix)
		if !ok {
			continue
		}
		slug := strings.TrimSuffix(filepath.Base(path), ".md")
		keySlug[r.Key] = slug
		if r.Kind == store.Log {
			logSlug[slug] = true
		}
		if d := laterOf(r.ValidFrom, r.UpdatedAt); d.After(dates[slug]) {
			dates[slug] = d
		}
	}
	return slugInfo{keySlug: keySlug, logSlug: logSlug, dates: dates, records: len(recs), logs: logs}, nil
}

func workers(n int) int {
	if n > 0 {
		return n
	}
	return min(8, runtime.NumCPU())
}

type parsedAll struct {
	prompts [][]Prompt
	counts  Counts
	gold    GoldCounts
	writes  []Access
}

func parseAll(files []string, local map[string]bool, lg Legacy, opt ParseOptions, nw int) (parsedAll, error) {
	results := make([][]Prompt, len(files))
	counts := make([]Counts, len(files))
	golds := make([]GoldCounts, len(files))
	writes := make([][]Access, len(files))
	errs := make([]error, len(files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range nw {
		wg.Go(func() {
			for i := range jobs {
				o := opt
				isLocal := local[filepath.Dir(files[i])]
				sub, err := SubagentScan(filepath.Join(strings.TrimSuffix(files[i], ".jsonl"), "subagents"), sessionOf(files[i]), lg, opt.Home, opt.Until)
				if err != nil {
					errs[i] = err
					continue
				}
				o.SubAccess = sub.Access
				if isLocal {
					o.SubagentUses = sub.Uses
				}
				f, err := os.Open(files[i])
				if err != nil {
					errs[i] = err
					continue
				}
				var parsed Parsed
				parsed, errs[i] = ParseTranscriptFull(f, sessionOf(files[i]), lg, o)
				_ = f.Close()
				results[i], counts[i], writes[i] = parsed.Prompts, parsed.Counts, parsed.Writes
				if isLocal {
					counts[i].SubagentFiles, counts[i].SubagentUses = sub.Files, len(o.SubagentUses)
				}
				golds[i] = parsed.Gold
				golds[i].add(sub.Gold)
				golds[i].SubFiles = sub.Files
			}
		})
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	var total parsedAll
	total.prompts = results
	for i := range files {
		if errs[i] != nil {
			return parsedAll{}, fmt.Errorf("%s: %w", files[i], errs[i])
		}
		total.counts.add(counts[i])
		total.gold.add(golds[i])
		total.writes = append(total.writes, writes[i]...)
	}
	return total, nil
}

func searchAll(ctx context.Context, s *store.Store, qs []*query, keySlug map[string]string, depth, nw int) error {
	type job struct{ q, r int }
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for range nw {
		wg.Go(func() {
			for j := range jobs {
				run := Runs[j.r]
				sq := store.Query{Text: qs[j.q].Query, Limit: depth, Mode: run.Mode}
				if run.Policy == "nolog" {
					sq.ExcludeKinds = []store.Kind{store.Log}
				}
				hits, err := s.Search(ctx, sq)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				out := make([]rankedHit, len(hits))
				for i, h := range hits {
					out[i] = rankedHit{slug: keySlug[h.Record.Key], hit: h}
				}
				qs[j.q].ranks[j.r] = out
			}
		})
	}
	for qi := range qs {
		for ri, run := range Runs {
			if run.Policy != "hook" {
				jobs <- job{qi, ri}
			}
		}
	}
	close(jobs)
	wg.Wait()
	return firstErr
}

func applyHook(qs []*query, keySlug map[string]string) {
	dst := runIndex(store.Fused, "hook")
	for _, q := range qs {
		q.ranks[dst] = injectedRanks(q, keySlug)
	}
}

func firstRanks(hits []rankedHit, gold []string) map[string]int {
	out := map[string]int{}
	for i, h := range hits {
		if slices.Contains(gold, h.slug) {
			if _, ok := out[h.slug]; !ok {
				out[h.slug] = i + 1
			}
		}
	}
	return out
}

type score struct {
	r4, r8, h4, h8, rr float64
	best               int
}

func scoreOf(hits []rankedHit, gold []string) score {
	var sc score
	if len(gold) == 0 {
		return sc
	}
	ranks := firstRanks(hits, gold)
	n4, n8 := 0, 0
	for _, r := range ranks {
		if r <= 4 {
			n4++
		}
		if r <= 8 {
			n8++
		}
		if sc.best == 0 || r < sc.best {
			sc.best = r
		}
	}
	sc.r4 = float64(n4) / float64(len(gold))
	sc.r8 = float64(n8) / float64(len(gold))
	if n4 > 0 {
		sc.h4 = 1
	}
	if n8 > 0 {
		sc.h8 = 1
	}
	if sc.best > 0 {
		sc.rr = 1 / float64(sc.best)
	}
	return sc
}

func unwritten(q *query) []string {
	var out []string
	for _, g := range q.gold {
		if !slices.Contains(q.Written, g) {
			out = append(out, g)
		}
	}
	return out
}

func metrics(qs []*query) []Metrics {
	var out []Metrics
	for _, goldSet := range []string{"read", "read-not-written"} {
		for ri, run := range Runs {
			m := Metrics{Run: run.Name(), Gold: goldSet}
			for _, q := range qs {
				gold := q.gold
				if goldSet != "read" {
					gold = unwritten(q)
				}
				if len(gold) == 0 {
					continue
				}
				sc := scoreOf(q.ranks[ri], gold)
				m.Prompts++
				m.Recall4 += sc.r4
				m.Recall8 += sc.r8
				m.Hit4 += sc.h4
				m.Hit8 += sc.h8
				m.MRR += sc.rr
			}
			if m.Prompts > 0 {
				n := float64(m.Prompts)
				m.Recall4, m.Recall8, m.Hit4, m.Hit8, m.MRR = m.Recall4/n, m.Recall8/n, m.Hit4/n, m.Hit8/n, m.MRR/n
			}
			out = append(out, m)
		}
	}
	return out
}

func quantiles(v []float64) Quantiles {
	if len(v) == 0 {
		return Quantiles{}
	}
	s := slices.Clone(v)
	slices.Sort(s)
	at := func(p float64) float64 { return s[int(p*float64(len(s)-1)+0.5)] }
	return Quantiles{N: len(s), P10: at(0.1), P50: at(0.5), P90: at(0.9)}
}

func calibrate(qs []*query) []Calibration {
	var out []Calibration
	for ri, run := range Runs {
		if run.Mode != store.Fused {
			continue
		}
		var gOv, oOv, gBM, oBM, tg, to []float64
		for _, q := range qs {
			for i, h := range q.ranks[ri] {
				if i >= 4 {
					break
				}
				isGold := slices.Contains(q.gold, h.slug)
				if isGold {
					gOv, gBM = append(gOv, h.hit.Overlap), append(gBM, h.hit.WordsBM25)
				} else {
					oOv, oBM = append(oOv, h.hit.Overlap), append(oBM, h.hit.WordsBM25)
				}
				if i == 0 {
					if isGold {
						tg = append(tg, h.hit.Overlap)
					} else {
						to = append(to, h.hit.Overlap)
					}
				}
			}
		}
		out = append(out, Calibration{
			Run: run.Name(), GoldOverlap: quantiles(gOv), OtherOverlap: quantiles(oOv),
			GoldWordsBM25: quantiles(gBM), OtherWordsBM25: quantiles(oBM),
			TopGoldOverlap: quantiles(tg), TopOtherOverlap: quantiles(to),
		})
	}
	return out
}

func tsvField(s string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
}

func boolCol(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func writeFile(path string, fn func(w *bufio.Writer) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if err := fn(w); err != nil {
		_ = f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeQueries(path string, qs []*query, logSlug map[string]bool) error {
	return writeFile(path, func(w *bufio.Writer) error {
		head := []string{"session", "turn", "hash", "prompt60", "queued", "short_tail", "query_chars", "gold_n", "gold_written_n", "gold_log_n", "gold"}
		for _, r := range Runs {
			n := strings.ReplaceAll(r.Name(), "/", "_")
			head = append(head, n+"_rank", n+"_r4", n+"_r8")
		}
		if _, err := fmt.Fprintln(w, strings.Join(head, "\t")); err != nil {
			return err
		}
		for _, q := range qs {
			nlog := 0
			for _, g := range q.gold {
				if logSlug[g] {
					nlog++
				}
			}
			row := []string{q.Session, fmt.Sprint(q.Turn), PromptHash(q.Text), tsvField(promptPrefix(q.Text)), boolCol(q.Queued),
				boolCol(q.ShortTail), fmt.Sprint(len([]rune(q.Query))), fmt.Sprint(len(q.gold)), fmt.Sprint(len(q.gold) - len(unwritten(q))),
				fmt.Sprint(nlog), strings.Join(q.gold, ",")}
			for ri := range Runs {
				sc := scoreOf(q.ranks[ri], q.gold)
				row = append(row, fmt.Sprint(sc.best), fmt.Sprintf("%.3f", sc.r4), fmt.Sprintf("%.3f", sc.r8))
			}
			if _, err := fmt.Fprintln(w, strings.Join(row, "\t")); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeHits(path string, qs []*query, limit int) error {
	return writeFile(path, func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "hash\trun\trank\tkey\tkind\tgold\tscore\twords_bm25\ttri_bm25\toverlap"); err != nil {
			return err
		}
		for _, q := range qs {
			hash := PromptHash(q.Text)
			for ri, run := range Runs {
				if run.Mode != store.Fused {
					continue
				}
				for i, h := range q.ranks[ri] {
					if i >= limit {
						break
					}
					_, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%.5f\t%.3f\t%.3f\t%.3f\n", hash, run.Name(), i+1, h.hit.Record.Key,
						h.hit.Record.Kind, boolCol(slices.Contains(q.gold, h.slug)), h.hit.Score, h.hit.WordsBM25, h.hit.TriBM25, h.hit.Overlap)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func writeSummary(path string, sum Summary) error {
	return writeFile(path, func(w *bufio.Writer) error {
		if _, err := fmt.Fprintln(w, "run\tgold\tprompts\trecall_at_4\trecall_at_8\thit_at_4\thit_at_8\tmrr"); err != nil {
			return err
		}
		for _, m := range sum.Metrics {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\n", m.Run, m.Gold, m.Prompts, m.Recall4, m.Recall8, m.Hit4, m.Hit8, m.MRR); err != nil {
				return err
			}
		}
		return nil
	})
}

type prepared struct {
	sum        Summary
	s          *store.Store
	keySlug    map[string]string
	logSlug    map[string]bool
	qs, negs   []*query
	cases      []*query
	caseSpecs  []Case
	local      []Prompt
	projectSet string
	nw         int
	gold       *GoldReport
}

func (p *prepared) id(q *query) string { return queryID(q) }

func prepare(ctx context.Context, e env.Env, cfg Config) (*prepared, error) {
	var p prepared
	sum := &p.sum
	if cfg.Out != "" {
		if err := os.MkdirAll(cfg.Out, 0o700); err != nil {
			return nil, err
		}
	}
	plan, err := legacy.Load(cfg.LegacyDir, legacy.Options{Project: cfg.Project})
	if err != nil {
		return nil, err
	}
	dirs, err := legacyDirs(cfg.LegacyDir)
	if err != nil {
		return nil, err
	}
	lg := Legacy{Dirs: dirs, Indexes: map[string]bool{}, Slugs: map[string]bool{}}
	for _, f := range plan.Files {
		lg.Slugs[f.Slug] = true
		if f.Index {
			lg.Indexes[f.Slug] = true
		}
	}
	files, err := expandGlob(cfg.Glob, e.Home())
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no transcripts match %s", cfg.Glob)
	}
	s, _, err := openDB(ctx, e, cfg)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = s.Close()
		}
	}()
	p.s = s
	sum.DB = s.Path()
	info, err := slugMap(ctx, s)
	if err != nil {
		return nil, err
	}
	keySlug, logSlug := info.keySlug, info.logSlug
	p.keySlug, p.logSlug = keySlug, logSlug
	sum.Records, sum.LogRecords, sum.Depth = info.records, info.logs, cfg.Depth
	known := map[string]bool{}
	for _, slug := range keySlug {
		known[slug] = true
	}
	p.nw = workers(cfg.Workers)
	projectDirs := map[string]bool{}
	for _, d := range dirs {
		projectDirs[filepath.Dir(d)] = true
	}
	start := time.Now()
	parsed, err := parseAll(files, projectDirs, lg, ParseOptions{MinChars: cfg.MinChars, Tail: cfg.Tail, MaxQuery: cfg.MaxQuery, Home: e.Home(), Until: cfg.Until}, p.nw)
	if err != nil {
		return nil, err
	}
	perFile := parsed.prompts
	sum.ParseSeconds = time.Since(start).Seconds()
	sum.Transcripts, sum.Counts = len(files), parsed.counts
	gopt := cfg.Gold.normalized()
	genv := newGoldEnv(info.dates, parsed.writes, gopt.Import, gopt.Scripts)
	sum.Settings, sum.Seed = cfg.Recall, cfg.Seed
	if !cfg.Until.IsZero() {
		sum.Until = cfg.Until.UTC().Format(time.RFC3339)
	}
	var qs []*query
	var local, all []Prompt
	for i, prompts := range perFile {
		isLocal := projectDirs[filepath.Dir(files[i])]
		for _, pr := range prompts {
			all = append(all, pr)
			if isLocal {
				local = append(local, pr)
			}
			q := &query{Prompt: pr, ranks: make([][]rankedHit, len(Runs))}
			for _, g := range gopt.pick(pr, genv).gold {
				if known[g] {
					q.gold = append(q.gold, g)
				} else {
					q.missing++
				}
			}
			sum.GoldMissing += q.missing
			if len(q.gold) == 0 {
				continue
			}
			sum.GoldFiles += len(q.gold)
			for _, g := range q.gold {
				if logSlug[g] {
					sum.GoldLog++
				}
			}
			uw := unwritten(q)
			sum.GoldWritten += len(q.gold) - len(uw)
			if len(uw) > 0 {
				sum.ReadOnlyGold++
			}
			if q.ShortTail {
				sum.ShortTail++
			}
			qs = append(qs, q)
		}
	}
	sum.Evaluated = len(qs)
	p.projectSet = SetProject
	if len(local) == 0 {
		local, p.projectSet = all, "all"
	}
	var negPool []Prompt
	for _, pr := range local {
		if !touched(pr) {
			negPool = append(negPool, pr)
		}
	}
	sum.NegativePool = len(negPool)
	p.gold, err = goldReport(cfg, gopt, genv, parsed, perFile, local, known, logSlug, len(negPool))
	if err != nil {
		return nil, err
	}
	sum.Gold = p.gold
	p.qs, p.local = qs, local
	p.negs = toQueries(sampleNegatives(negPool, cfg.Negatives, cfg.Seed))
	if cfg.Cases != "" {
		if p.caseSpecs, err = loadCases(cfg.Cases); err != nil {
			return nil, err
		}
		p.cases = make([]*query, len(p.caseSpecs))
		for i, c := range p.caseSpecs {
			p.cases[i] = &query{Prompt: Prompt{Session: CaseSession, Turn: i + 1, Text: c.Prompt, Query: clip(c.Prompt, cfg.MaxQuery)}, caseName: c.Name}
		}
	}
	ok = true
	return &p, nil
}

func Evaluate(ctx context.Context, e env.Env, cfg Config) (Summary, error) {
	p, err := prepare(ctx, e, cfg)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = p.s.Close() }()
	sum, s, qs, negs, keySlug, logSlug, nw := p.sum, p.s, p.qs, p.negs, p.keySlug, p.logSlug, p.nw
	start := time.Now()
	if err := searchAll(ctx, s, qs, keySlug, cfg.Depth, nw); err != nil {
		return sum, err
	}
	if err := hookSearch(ctx, s, append(slices.Clone(qs), negs...), cfg.Recall, nw); err != nil {
		return sum, err
	}
	applyHook(qs, keySlug)
	sum.Hook = []HookStats{hookStats(SetGold, qs, outcomes(qs), keySlug), hookStats(SetNegative, negs, outcomes(negs), keySlug)}
	if cfg.AllPrompts {
		proj := toQueries(p.local)
		if err := hookSearch(ctx, s, proj, cfg.Recall, nw); err != nil {
			return sum, err
		}
		sum.Hook = append(sum.Hook, hookStats(p.projectSet, proj, outcomes(proj), keySlug))
	}
	if len(p.cases) > 0 {
		if err := hookSearch(ctx, s, p.cases, cfg.Recall, nw); err != nil {
			return sum, err
		}
		for i, c := range p.caseSpecs {
			sum.Cases = append(sum.Cases, judgeCase(c, p.cases[i]))
		}
	}
	if cfg.Sweep != "" {
		rows := sweep(qs, negs, cfg.Recall, keySlug, SweepGrid(), nw)
		if err := writeSweep(cfg.Sweep, rows); err != nil {
			return sum, err
		}
		sum.SweepRows = len(rows)
		for i := range rows {
			if rows[i].Knee {
				sum.SweepKnee = &rows[i]
			}
		}
		sum.CrossFit = crossFit(rows)
	}
	sum.SearchSeconds = time.Since(start).Seconds()
	sum.Metrics = metrics(qs)
	sum.Calibration = calibrate(qs)
	if err := writeQueries(filepath.Join(cfg.Out, "queries.tsv"), qs, logSlug); err != nil {
		return sum, err
	}
	if err := writeHits(filepath.Join(cfg.Out, "hits.tsv"), qs, cfg.Limit); err != nil {
		return sum, err
	}
	if err := writeHook(filepath.Join(cfg.Out, "hook.tsv"), filepath.Join(cfg.Out, "candidates.tsv"), map[string][]*query{SetGold: qs, SetNegative: negs}, keySlug); err != nil {
		return sum, err
	}
	if err := writeSummary(filepath.Join(cfg.Out, "summary.tsv"), sum); err != nil {
		return sum, err
	}
	if err := writeHookSummary(filepath.Join(cfg.Out, "hook-summary.tsv"), sum.Hook); err != nil {
		return sum, err
	}
	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return sum, err
	}
	return sum, os.WriteFile(filepath.Join(cfg.Out, "summary.json"), append(data, '\n'), 0o600)
}

func toQueries(ps []Prompt) []*query {
	out := make([]*query, len(ps))
	for i, p := range ps {
		out[i] = &query{Prompt: p}
	}
	return out
}

func printSummary(w io.Writer, sum Summary) {
	c := sum.Counts
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	p("transcripts %d (%d lines, %d unparsable, %d sidechain entries skipped, %d entries after --until)\n", sum.Transcripts, c.Lines, c.BadLines, c.Sidechain, c.Late)
	p("turns %d: genuine prompts %d (%d queued mid-turn); excluded not human %d, tagged %d, slash %d, interrupted or summary %d, empty %d\n",
		c.Boundaries, c.Genuine, c.Queued, c.NotHuman, c.Tagged, c.Slash, c.Interrupted, c.Empty)
	p("legacy reads %d, index reads %d, other projects' memory reads %d, gold reads outside genuine turns %d\n",
		c.MemoryReads, c.IndexReads, c.OtherMemReads, c.OrphanReads)
	p("other memory use: %d tool calls in main sessions, %d in %d subagent files of the project; %d genuine prompts without reads used memory and stay out of the negative pool\n",
		c.OtherUses, c.SubagentUses, c.SubagentFiles, c.PromptsUsing)
	p("db %s: %d records (%d log)\n", sum.DB, sum.Records, sum.LogRecords)
	p("prompts with gold %d (%d with assistant tail appended): gold files %d, missing from db %d, stored as log %d, written in the same turn %d\n",
		sum.Evaluated, sum.ShortTail, sum.GoldFiles, sum.GoldMissing, sum.GoldLog, sum.GoldWritten)
	if g := sum.Gold; g != nil {
		printGold(w, g)
	}
	p("parse %.1fs, search %.1fs\n\n", sum.ParseSeconds, sum.SearchSeconds)
	p("%-14s %-17s %7s %7s %7s %7s %7s %7s\n", "run", "gold", "prompts", "R@4", "R@8", "hit@4", "hit@8", fmt.Sprintf("MRR@%d", sum.Depth))
	for _, m := range sum.Metrics {
		p("%-14s %-17s %7d %7.3f %7.3f %7.3f %7.3f %7.3f\n", m.Run, m.Gold, m.Prompts, m.Recall4, m.Recall8, m.Hit4, m.Hit8, m.MRR)
	}
	p("\ncalibration over top-4 hits: n, p10 / p50 / p90\n")
	q := func(x Quantiles) string { return fmt.Sprintf("%d, %.3f / %.3f / %.3f", x.N, x.P10, x.P50, x.P90) }
	for _, cal := range sum.Calibration {
		p("%s overlap gold %s; other %s\n", cal.Run, q(cal.GoldOverlap), q(cal.OtherOverlap))
		p("%s words_bm25 gold %s; other %s\n", cal.Run, q(cal.GoldWordsBM25), q(cal.OtherWordsBM25))
		p("%s top-1 overlap gold %s; other %s\n", cal.Run, q(cal.TopGoldOverlap), q(cal.TopOtherOverlap))
	}
	st := sum.Settings
	p("\nhook path (%s, at most %d records, %d chars):\n", describeGate(st), st.Limit, st.MaxChars)
	if n := promptsOf(sum.Hook, SetNegative); n < sum.NegativePool {
		p("negative set: %d sampled (seed %d) of %d genuine prompts without memory use\n", n, sum.Seed, sum.NegativePool)
	} else {
		p("negative set: all %d genuine prompts without memory use\n", n)
	}
	p("%-9s %7s %9s %7s %8s %6s %8s %10s %8s %7s %9s\n", "set", "prompts", "injecting", "rate", "rec/pr", "cut", "chars/pr", "chars/inj", "recall", "hit", "precision")
	for _, h := range sum.Hook {
		p("%-9s %7d %9d %7.3f %8.3f %6d %8.1f %10.1f %8.3f %7.3f %9.3f\n", h.Set, h.Prompts, h.Injecting, h.InjectRate, h.MeanRecords, h.Cut,
			h.MeanChars, h.MeanCharsInjecting, h.Recall, h.Hit, h.Precision)
	}
	if len(sum.Cases) > 0 {
		pass := 0
		for _, c := range sum.Cases {
			if c.Pass {
				pass++
			}
		}
		p("\ncases %d of %d pass\n", pass, len(sum.Cases))
		for _, c := range sum.Cases {
			verdict := "PASS"
			if !c.Pass {
				verdict = "FAIL"
			}
			want := "nothing"
			if !c.None {
				want = strings.Join(c.Want, ",")
			}
			got := strings.Join(c.Injected, ",")
			if got == "" {
				got = "nothing"
			}
			p("%s %s: want %s, injected %s (%d chars)\n", verdict, c.Name, want, got, c.Chars)
			for _, t := range c.Top {
				p("     %s score %.4f cover %.3f shared %d/%d topic %d\n", t.Key, t.Score, t.Cover, t.Shared, t.Terms, t.Topic)
			}
		}
	}
	if sum.SweepRows > 0 {
		p("\nsweep: %d settings\n", sum.SweepRows)
		if k := sum.SweepKnee; k != nil {
			p("knee over both folds: %s: recall %.4f, false injection %.4f\n", gateString(k.Gate), k.Gold.Recall, k.Negative.InjectRate)
		}
		for _, cf := range sum.CrossFit {
			p("knee of fold %d (%s): recall %.4f, false injection %.4f there; on fold %d recall %.4f, false injection %.4f\n",
				cf.PickedOn, gateString(cf.Gate), cf.InFold.Recall, cf.InFold.FalseRate, 1-cf.PickedOn, cf.HeldOut.Recall, cf.HeldOut.FalseRate)
			p("     fold %d's own knee (%s): recall %.4f, false injection %.4f; best recall there at false injection <= %.4f: %.4f\n",
				1-cf.PickedOn, gateString(cf.HeldKnee), cf.HeldBest.Recall, cf.HeldBest.FalseRate, cf.HeldOut.FalseRate, cf.HeldAtFIR)
		}
	}
}

func describeGate(st recall.Settings) string {
	return fmt.Sprintf("fused, no log, pool %d, %s", st.Pool, gateString(st.Gate))
}

func gateString(g recall.Gate) string {
	lead := "off"
	if g.Lead {
		lead = fmt.Sprintf("shared >= %d", g.LeadShared)
	}
	return fmt.Sprintf("cover >= %.2f, shared >= %d, topic >= %d, lead %s", g.MinCover, g.MinShared, g.MinTopic, lead)
}

func promptsOf(hs []HookStats, set string) int {
	for _, h := range hs {
		if h.Set == set {
			return h.Prompts
		}
	}
	return 0
}

func failedCases(cs []CaseResult) int {
	n := 0
	for _, c := range cs {
		if !c.Pass {
			n++
		}
	}
	return n
}

const usage = "usage: mem eval --legacy-dir D --out DIR [--transcripts GLOB] [--project P] [--db PATH] [--until TIME]\n" +
	"       [--limit N] [--depth N] [--tail N] [--min-chars N] [--max-query N] [--workers N]\n" +
	"       [--gate-cover F] [--gate-shared N] [--gate-topic N] [--gate-lead=BOOL] [--gate-lead-shared N] [--gate-pool N] [--gate-limit N] [--gate-chars N]\n" +
	"       [--negatives N] [--seed N] [--cases FILE] [--sweep FILE] [--all-prompts=false] [--json]\n" +
	"       mem eval --legacy-dir D --export-texts DIR [--cases FILE] [--transcripts GLOB] [--db PATH] [--until TIME]\n" +
	"       mem eval --legacy-dir D --out DIR --vectors DIR [--docs-vectors DIR] [--vec-weight F] [--vec-depth N] [--allow-other-texts] [--db PATH] [--until TIME]\n" +
	"       [--logs-gate [--folds N] [--fir-budgets LIST] [--pools LIST] [--lex-regular=BOOL] [--lex-sweep]] [--allow-extra-vectors]\n" +
	"       gold: [--gold-tier 0|1|2] [--gold-direct=BOOL] [--gold-bash] [--gold-subagent] [--gold-scripts] [--gold-dates db|events|both] [--import-time TIME]\n" +
	"       [--gold-matrix FILE] [--write-events FILE]\n" +
	"       mem eval failures --transcripts GLOB --turns FILE --out DIR [--since YYYY-MM-DD] [--max N] [--horizon N] [--dry-run] [--db PATH] [-- HOOK FLAGS...]\n"

func evalFail(e env.Env, err error) int {
	_, _ = fmt.Fprintln(e.Stderr, "mem eval:", err)
	if errors.Is(err, store.ErrInvalid) {
		return 2
	}
	return 1
}

func Run(args []string, e env.Env) int {
	if len(args) > 0 && args[0] == "failures" {
		return runFailures(args[1:], e)
	}
	fs := flag.NewFlagSet("mem eval", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(e.Stderr, usage)
		fs.PrintDefaults()
	}
	var cfg Config
	fs.StringVar(&cfg.Glob, "transcripts", DefaultGlob, "Claude Code main-session transcripts")
	fs.StringVar(&cfg.LegacyDir, "legacy-dir", "", "legacy memory directory, read only")
	fs.StringVar(&cfg.Out, "out", "", "output directory for eval.db and the TSV reports")
	fs.StringVar(&cfg.Project, "project", DefaultProject, "project scope for the fresh import")
	fs.StringVar(&cfg.DB, "db", e.DBFlag, "evaluate an existing imported database instead of importing into DIR/eval.db")
	until := fs.String("until", "", "ignore transcript entries stamped after this RFC 3339 time")
	fs.IntVar(&cfg.Limit, "limit", DefaultLimit, "hits per query written to hits.tsv")
	fs.IntVar(&cfg.Depth, "depth", DefaultDepth, "ranking depth for MRR")
	fs.IntVar(&cfg.Tail, "tail", DefaultTail, "characters of the last assistant message appended to short prompts")
	fs.IntVar(&cfg.MinChars, "min-chars", DefaultMinChars, "prompts shorter than this get the assistant tail")
	fs.IntVar(&cfg.MaxQuery, "max-query", DefaultMaxQuery, "prompt characters kept in the query")
	fs.IntVar(&cfg.Workers, "workers", 0, "parallel workers, 0 = min(8, CPUs)")
	d := recall.DefaultSettings()
	cfg.Recall = d
	fs.Float64Var(&cfg.Recall.MinCover, "gate-cover", d.MinCover, "hook gate: least share of the query stems found in a record")
	fs.IntVar(&cfg.Recall.MinShared, "gate-shared", d.MinShared, "hook gate: least shared stems")
	fs.IntVar(&cfg.Recall.MinTopic, "gate-topic", d.MinTopic, "hook gate: least key or title stems in the query")
	fs.BoolVar(&cfg.Recall.Lead, "gate-lead", d.Lead, "hook gate: top hit passes without --gate-cover when it ranks first in both tables and the runner-up has no trigram match")
	fs.IntVar(&cfg.Recall.LeadShared, "gate-lead-shared", d.LeadShared, "hook gate: least shared stems of a top hit taken by --gate-lead")
	fs.IntVar(&cfg.Recall.Pool, "gate-pool", d.Pool, "hook gate: fused hits examined")
	fs.IntVar(&cfg.Recall.Limit, "gate-limit", d.Limit, "hook gate: records emitted")
	fs.IntVar(&cfg.Recall.MaxChars, "gate-chars", d.MaxChars, "hook packer: budget of the emitted context in chars")
	fs.IntVar(&cfg.Negatives, "negatives", DefaultNegatives, "genuine prompts without memory use sampled as the negative set, 0 = all of them")
	fs.Uint64Var(&cfg.Seed, "seed", DefaultSeed, "seed of the negative sample")
	fs.StringVar(&cfg.Cases, "cases", "", "JSON list of handcrafted prompts with the keys the hook must inject, or none")
	fs.StringVar(&cfg.Sweep, "sweep", "", "write a gate sweep over the gold and negative sets to this TSV")
	fs.StringVar(&cfg.ExportTexts, "export-texts", "", "write queries.jsonl and docs.jsonl for mem embed into this directory and stop")
	fs.StringVar(&cfg.Vectors, "vectors", "", "evaluate the query and document vectors written by mem embed in this directory instead of the lexical sweep")
	fs.StringVar(&cfg.DocsVectors, "docs-vectors", "", "take the document vectors from this mem embed directory instead of --vectors")
	fs.Float64Var(&cfg.VecWeight, "vec-weight", DefaultVecWeight, "weight of the vector list in the fused RRF, the words list has 1 and the trigram list 0.3")
	fs.IntVar(&cfg.VecDepth, "vec-depth", DefaultVecDepth, "ranking depth of the vector list before fusion")
	fs.BoolVar(&cfg.LexicalRef, "lexical-ref", true, "with --vectors, also run the lexical hook gate over the same prompts as the reference")
	fs.BoolVar(&cfg.AllowOtherTexts, "allow-other-texts", false, "with --vectors, accept vectors made from texts other than this run exports")
	fs.BoolVar(&cfg.LogsGate, "logs-gate", false, "with --vectors, simulate a hook that searches regular records and log chunks and injects under separate gates, cross-fitted by session")
	fs.IntVar(&cfg.Folds, "folds", DefaultFolds, "with --logs-gate, number of session folds of the cross-fit")
	budgets := fs.String("fir-budgets", DefaultBudgets, "with --logs-gate, comma-separated false-injection budgets the gate is picked under")
	pools := fs.String("pools", strings.Join(AllPools, ","), "with --logs-gate, comma-separated candidate pools: reg, fused-reg, all, fused-all")
	fs.BoolVar(&cfg.LexRegular, "lex-regular", true, "with --logs-gate, also try the shipped lexical gate for regular records with log chunks added by vector")
	fs.BoolVar(&cfg.LexSweep, "lex-sweep", false, "with --logs-gate, also cross-fit the lexical hook gate over the sweep grid as the variant lex-sweep")
	fs.BoolVar(&cfg.AllPrompts, "all-prompts", true, "also run the hook path over every genuine prompt of the project to measure injection volume")
	cfg.Gold = DefaultGold()
	fs.IntVar(&cfg.Gold.Tier, "gold-tier", 0, "gold tier: 0 every read, 1 drops files written in the same turn, 2 also drops files written after the prompt before the import")
	fs.BoolVar(&cfg.Gold.Direct, "gold-direct", true, "gold source: Read calls of the prompt's turn")
	fs.BoolVar(&cfg.Gold.Bash, "gold-bash", false, "gold source: Bash commands that name a memory file explicitly (cat, head, tail, less, bat, sed, awk, rg, grep)")
	fs.BoolVar(&cfg.Gold.Scripts, "gold-scripts", false, "tiers 1 and 2 also treat an interpreter script that names a memory file and writes somewhere as a write to it (over-approximates)")
	fs.BoolVar(&cfg.Gold.Sub, "gold-subagent", false, "gold source: Read calls in subagent transcripts, attributed to the parent turn by time")
	fs.StringVar(&cfg.Gold.Dates, "gold-dates", DatesDB, "tier 2 content date: db (valid_from of the records), events (last write in the transcripts) or both")
	importTime := fs.String("import-time", "", "RFC 3339 time the --db was imported, bounds the write events of tier 2, default the mtime of --db")
	fs.StringVar(&cfg.GoldMatrix, "gold-matrix", "", "write the gold counts of every tier and source combination to this TSV")
	fs.StringVar(&cfg.WriteEvents, "write-events", "", "write every write to a legacy memory file found in the transcripts to this TSV")
	fs.BoolVar(&cfg.AllowExtraVectors, "allow-extra-vectors", false, "with --vectors, accept query vectors of prompts that are not in this run's sets (a stricter gold tier)")
	asJSON := fs.Bool("json", false, "print the summary as JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem eval: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if cfg.LegacyDir == "" || cfg.Out == "" && (cfg.ExportTexts == "" || cfg.DB == "") {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval: --legacy-dir and --out are required (--out is not needed for --export-texts together with --db)")
		return 2
	}
	if cfg.ExportTexts != "" && cfg.Vectors != "" || cfg.DocsVectors != "" && cfg.Vectors == "" {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval: --export-texts and --vectors exclude each other, --docs-vectors needs --vectors")
		return 2
	}
	if *until != "" {
		t, err := time.Parse(time.RFC3339, *until)
		if err != nil {
			_, _ = fmt.Fprintf(e.Stderr, "mem eval: --until: %v\n", err)
			return 2
		}
		cfg.Until = t
	}
	if err := cfg.Gold.Validate(); err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval:", err)
		return 2
	}
	if *importTime != "" {
		t, err := time.Parse(time.RFC3339, *importTime)
		if err != nil {
			_, _ = fmt.Fprintf(e.Stderr, "mem eval: --import-time: %v\n", err)
			return 2
		}
		cfg.Gold.Import = t
	} else if cfg.DB != "" {
		if info, err := os.Stat(cfg.DB); err == nil {
			cfg.Gold.Import = info.ModTime().UTC().Truncate(time.Second)
		}
	}
	if !(cfg.VecWeight >= 0) || math.IsInf(cfg.VecWeight, 0) || cfg.VecDepth < cfg.Recall.Pool {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval: --vec-weight must be a finite number, not negative, and --vec-depth at least --gate-pool")
		return 2
	}
	if err := logsGateFlags(fs, &cfg, *budgets, *pools); err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval:", err)
		return 2
	}
	r := cfg.Recall
	if cfg.Depth < max(8, r.Pool) || cfg.Limit < 1 || r.Limit < 1 || r.Pool < 1 || r.MaxChars < 1 || cfg.Tail < 0 || cfg.MinChars < 0 ||
		cfg.MaxQuery < 0 || cfg.Workers < 0 || cfg.Negatives < 0 || r.MinShared < 0 || r.MinTopic < 0 || r.MinCover < 0 || r.LeadShared < 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem eval: --depth must be at least 8 and at least --gate-pool; --limit, --gate-limit, --gate-pool and --gate-chars at least 1; the rest non-negative")
		return 2
	}
	cfg.Out = expandHome(cfg.Out, e.Home())
	cfg.ExportTexts = expandHome(cfg.ExportTexts, e.Home())
	cfg.Vectors = expandHome(cfg.Vectors, e.Home())
	cfg.DocsVectors = expandHome(cfg.DocsVectors, e.Home())
	cfg.LegacyDir = expandHome(cfg.LegacyDir, e.Home())
	if cfg.Cases != "" {
		cfg.Cases = expandHome(cfg.Cases, e.Home())
	}
	cfg.GoldMatrix = expandHome(cfg.GoldMatrix, e.Home())
	cfg.WriteEvents = expandHome(cfg.WriteEvents, e.Home())
	if cfg.Sweep != "" {
		cfg.Sweep = expandHome(cfg.Sweep, e.Home())
	}
	if cfg.Vectors != "" {
		sum, err := EvaluateVectors(context.Background(), e, cfg)
		if err != nil {
			return evalFail(e, err)
		}
		if *asJSON {
			enc := json.NewEncoder(e.Stdout)
			enc.SetIndent("", "  ")
			sum.Rows = nil
			if err := enc.Encode(sum); err != nil {
				return 1
			}
		} else {
			printVectorSummary(e.Stdout, sum)
		}
		return 0
	}
	if cfg.ExportTexts != "" {
		c, err := ExportTexts(context.Background(), e, cfg)
		if err != nil {
			return evalFail(e, err)
		}
		_, _ = fmt.Fprintf(e.Stdout, "queries %d (gold %d, negative %d, case %d), docs %d, written to %s\n", c.Gold+c.Negative+c.Case, c.Gold, c.Negative, c.Case, c.Docs, cfg.ExportTexts)
		return 0
	}
	sum, err := Evaluate(context.Background(), e, cfg)
	if err != nil {
		return evalFail(e, err)
	}
	if *asJSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sum); err != nil {
			return 1
		}
	} else {
		printSummary(e.Stdout, sum)
	}
	if n := failedCases(sum.Cases); n > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem eval: %d of %d cases fail\n", n, len(sum.Cases))
		return 1
	}
	return 0
}

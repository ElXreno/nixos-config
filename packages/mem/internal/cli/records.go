package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"mem/internal/env"
	"mem/internal/legacy"
	"mem/internal/store"
)

type recordScopes struct {
	Hosts    []string `json:"hosts"`
	Projects []string `json:"projects"`
	Tools    []string `json:"tools"`
}

type recordLink struct {
	Dst string `json:"dst"`
	Rel string `json:"rel"`
}

type recordEvidence struct {
	Slug  string `json:"slug"`
	Quote string `json:"quote"`
}

type recordInput struct {
	Key          string           `json:"key"`
	Kind         string           `json:"kind"`
	Core         bool             `json:"core"`
	Title        string           `json:"title"`
	Body         string           `json:"body"`
	Scopes       recordScopes     `json:"scopes"`
	Triggers     []store.Trigger  `json:"triggers"`
	TTLDays      *int             `json:"ttl_days"`
	VerifyCmd    *string          `json:"verify_cmd"`
	Status       string           `json:"status"`
	SupersededBy *string          `json:"superseded_by"`
	ValidFrom    string           `json:"valid_from"`
	Links        []recordLink     `json:"links"`
	DerivedFrom  []string         `json:"derived_from"`
	Evidence     []recordEvidence `json:"evidence"`
	Source       string           `json:"source"`

	file string
	kind store.Kind
	born time.Time
	supe string
}

type distinctDecision struct {
	Key  string   `json:"key"`
	With []string `json:"with"`
	Why  string   `json:"why"`
}

type conflictCandidate struct {
	Key        string  `json:"key"`
	Title      string  `json:"title"`
	Similarity float64 `json:"similarity"`
	Shared     int     `json:"shared"`
}

type conflict struct {
	Key        string              `json:"key"`
	Title      string              `json:"title"`
	Duplicates []conflictCandidate `json:"duplicates"`
	Reason     string              `json:"reason,omitempty"`
}

type importSummary struct {
	Records    int        `json:"records"`
	Created    int        `json:"created"`
	Distinct   int        `json:"distinct"`
	Superseded []string   `json:"superseded,omitempty"`
	Conflicts  []conflict `json:"conflicts,omitempty"`
	Failed     []string   `json:"failed,omitempty"`
	Existing   []string   `json:"existing,omitempty"`
	DryRun     bool       `json:"dry_run,omitempty"`
}

func readRecordFiles(paths []string) ([]*recordInput, error) {
	var out []*recordInput
	seen := map[string]string{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for n := 1; sc.Scan(); n++ {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			var r recordInput
			if err := json.Unmarshal(line, &r); err != nil {
				return nil, invalidRecord("%s:%d: %v", p, n, err)
			}
			r.file = p
			if r.Source == "" {
				r.Source = "migrate:" + baseName(p)
			}
			if prev, ok := seen[r.Key]; ok {
				return nil, invalidRecord("%s:%d: duplicate key %s (first in %s)", p, n, r.Key, prev)
			}
			seen[r.Key] = fmt.Sprintf("%s:%d", p, n)
			out = append(out, &r)
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, ".jsonl")
}

func invalidRecord(format string, a ...any) error {
	return fmt.Errorf("%w: %s", store.ErrInvalid, fmt.Sprintf(format, a...))
}

func (r *recordInput) scopes(extra []string) []string {
	var out []string
	for _, h := range r.Scopes.Hosts {
		out = append(out, store.HostScope(h))
	}
	for _, p := range r.Scopes.Projects {
		out = append(out, store.ProjectScope(p))
	}
	if len(out) == 0 || r.Core {
		out = append(out, store.GlobalScope)
	}
	return append(out, extra...)
}

func (r *recordInput) record(extra []string) store.Record {
	rec := store.Record{Key: r.Key, Kind: r.kind, Scopes: r.scopes(extra), Core: r.Core, Title: r.Title, Body: r.Body, Triggers: r.Triggers, Source: r.Source}
	if r.kind == store.State && r.TTLDays != nil {
		rec.TTLDays = *r.TTLDays
	}
	return rec
}

func validateRecords(recs []*recordInput, extra []string) error {
	keys := map[string]*recordInput{}
	for _, r := range recs {
		keys[r.Key] = r
	}
	for _, r := range recs {
		k, err := store.ParseKind(r.Kind)
		if err != nil {
			return invalidRecord("%s: %v", r.Key, err)
		}
		if k == store.Log {
			return invalidRecord("%s: kind log is written by import-legacy", r.Key)
		}
		r.kind = k
		if err := r.record(extra).Validate(); err != nil {
			return invalidRecord("%s: %v", r.Key, err)
		}
		t, err := time.Parse(time.DateOnly, r.ValidFrom)
		if err != nil {
			return invalidRecord("%s: valid_from %q is not YYYY-MM-DD", r.Key, r.ValidFrom)
		}
		r.born = t
		switch r.Status {
		case "current", "":
			if r.SupersededBy != nil && *r.SupersededBy != "" {
				return invalidRecord("%s: current record has superseded_by", r.Key)
			}
		case "superseded":
			if r.SupersededBy == nil || *r.SupersededBy == "" {
				return invalidRecord("%s: superseded record has no superseded_by", r.Key)
			}
			if _, ok := keys[*r.SupersededBy]; !ok {
				return invalidRecord("%s: superseded_by %s is not in the input", r.Key, *r.SupersededBy)
			}
			if *r.SupersededBy == r.Key {
				return invalidRecord("%s: supersedes itself", r.Key)
			}
			r.supe = *r.SupersededBy
		default:
			return invalidRecord("%s: status %q is not current or superseded", r.Key, r.Status)
		}
		for _, l := range r.Links {
			if l.Rel != string(store.Relates) && l.Rel != string(store.Supersedes) {
				return invalidRecord("%s: link relation %q is not relates or supersedes", r.Key, l.Rel)
			}
			if l.Dst == "" || l.Dst == r.Key {
				return invalidRecord("%s: bad link target %q", r.Key, l.Dst)
			}
		}
		if len(r.DerivedFrom) == 0 {
			return invalidRecord("%s: derived_from is empty", r.Key)
		}
	}
	return nil
}

func supersessionPlan(recs []*recordInput) (map[string][]string, []*recordInput, error) {
	byKey := map[string]*recordInput{}
	for _, r := range recs {
		byKey[r.Key] = r
	}
	by := map[string]string{}
	for _, r := range recs {
		if r.supe != "" {
			by[r.Key] = r.supe
		}
	}
	for _, r := range recs {
		for _, l := range r.Links {
			if l.Rel != string(store.Supersedes) || r.supe == l.Dst {
				continue
			}
			if _, ok := byKey[l.Dst]; !ok {
				return nil, nil, invalidRecord("%s: supersedes %s, which is not in the input", r.Key, l.Dst)
			}
			if _, ok := by[l.Dst]; !ok {
				by[l.Dst] = r.Key
			}
		}
	}
	sup := map[string][]string{}
	for old, repl := range by {
		sup[repl] = append(sup[repl], old)
	}
	for _, v := range sup {
		sort.Strings(v)
	}
	var order []*recordInput
	state := map[string]int{}
	var visit func(r *recordInput) error
	visit = func(r *recordInput) error {
		switch state[r.Key] {
		case 2:
			return nil
		case 1:
			return invalidRecord("supersession cycle through %s", r.Key)
		}
		state[r.Key] = 1
		for _, o := range sup[r.Key] {
			if err := visit(byKey[o]); err != nil {
				return err
			}
		}
		state[r.Key] = 2
		order = append(order, r)
		return nil
	}
	for _, r := range recs {
		if err := visit(r); err != nil {
			return nil, nil, err
		}
	}
	return sup, order, nil
}

func quoteNote(quotes []string) string {
	if len(quotes) == 0 {
		return ""
	}
	parts := make([]string, len(quotes))
	for i, q := range quotes {
		parts[i] = `"` + q + `"`
	}
	return "evidence: " + strings.Join(parts, " | ")
}

func (r *recordInput) links(ctx context.Context, s *store.Store, superseding []string) ([]store.Link, error) {
	var out []store.Link
	for _, l := range r.Links {
		if l.Rel == string(store.Supersedes) && slices.Contains(superseding, l.Dst) {
			continue
		}
		rel := store.Rel(l.Rel)
		if rel == store.Supersedes && r.supe == l.Dst {
			rel = store.Relates
		}
		out = append(out, store.Link{Dst: l.Dst, Rel: rel})
	}
	quotes := map[string][]string{}
	var slugs []string
	for _, e := range r.Evidence {
		if _, ok := quotes[e.Slug]; !ok {
			slugs = append(slugs, e.Slug)
		}
		quotes[e.Slug] = append(quotes[e.Slug], e.Quote)
	}
	derived := map[string]bool{}
	for _, d := range r.DerivedFrom {
		derived[d] = true
		if _, ok := quotes[d]; !ok {
			slugs = append(slugs, d)
		}
	}
	for _, slug := range slugs {
		dst := legacy.LogKey(slug)
		if _, err := s.Get(ctx, dst); err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
			dst = slug
		}
		rel := store.Relates
		if derived[slug] {
			rel = store.DerivedFrom
		}
		out = append(out, store.Link{Dst: dst, Rel: rel, Note: quoteNote(quotes[slug])})
	}
	return out, nil
}

func readDecisions(path string) (map[string]distinctDecision, error) {
	out := map[string]distinctDecision{}
	if path == "" {
		return out, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for n, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var d distinctDecision
		if err := json.Unmarshal(line, &d); err != nil {
			return nil, invalidRecord("%s:%d: %v", path, n+1, err)
		}
		if d.Key == "" || strings.TrimSpace(d.Why) == "" {
			return nil, invalidRecord("%s:%d: a distinct decision needs key and why", path, n+1)
		}
		out[d.Key] = d
	}
	return out, nil
}

func cmdImportRecords(e env.Env, args []string) int {
	f := newFlags(&e, "import-records", "[--distinct FILE] [--conflicts FILE] [--why TEXT] [--dry-run] FILE.jsonl...", true)
	distinct := f.String("distinct", "", "JSONL of {key, with, why}: keys the gate flags that are distinct from their candidates")
	conflictsPath := f.String("conflicts", "", "write the gate rejections as JSONL to this file")
	why := f.String("why", "import", "reason kept in history")
	relatedPath := f.String("related", "", "write each created record with its check-for-conflict neighbours as JSONL to this file")
	var extraScopes multi
	f.Var(&extraScopes, "scope", "scope added to every record, for example project:nixos-config; repeatable")
	dry := f.Bool("dry-run", false, "validate and print the creation order, write nothing")
	dup := f.Float64("dup", store.DefaultDup, "duplicate similarity threshold")
	rel := f.Float64("rel", store.DefaultRel, "similarity from which a neighbour is printed as check for conflict; one that shares two words of key and title is printed from 0.1")
	minShared := f.Int("min-shared", store.DefaultMinShared, "stems a duplicate must share at least")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) == 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem import-records: needs at least one JSONL file")
		return 2
	}
	if len(extraScopes) > 0 {
		ns, err := store.NormalizeScopes(extraScopes)
		if err != nil {
			return fail(e, err)
		}
		extraScopes = ns
	}
	recs, err := readRecordFiles(pos)
	if err == nil {
		err = validateRecords(recs, extraScopes)
	}
	var sup map[string][]string
	var order []*recordInput
	if err == nil {
		sup, order, err = supersessionPlan(recs)
	}
	if err != nil {
		return fail(e, err)
	}
	decisions, err := readDecisions(*distinct)
	if err != nil {
		return fail(e, err)
	}
	var relatedBuf bytes.Buffer
	sum := importSummary{Records: len(recs), DryRun: *dry}
	if *dry {
		return printImport(e, f.asJSON(), sum)
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	gate := store.Gate{Dup: *dup, Rel: *rel, MinShared: *minShared}
	for _, r := range order {
		links, err := r.links(ctx, s, sup[r.Key])
		if err != nil {
			return fail(e, err)
		}
		rec := r.record(extraScopes)
		req := store.PutRequest{
			Key: r.Key, Kind: r.kind, Scopes: rec.Scopes, Core: &r.Core, Title: &r.Title, Body: &r.Body,
			Triggers: &rec.Triggers, Source: &r.Source, Supersedes: sup[r.Key], Why: *why, Gate: gate, AsOf: r.born, Links: links,
		}
		if r.kind == store.State {
			ttl := rec.TTLDays
			req.TTLDays = &ttl
		}
		if r.VerifyCmd != nil && *r.VerifyCmd != "" {
			req.VerifyCmd = r.VerifyCmd
		}
		res, err := s.Put(ctx, req)
		var ge *store.GateError
		if errors.As(err, &ge) {
			if d, ok := decisions[r.Key]; ok {
				req.Distinct = d.With
				if len(req.Distinct) == 0 {
					for _, c := range ge.Duplicates {
						req.Distinct = append(req.Distinct, c.Record.Key)
					}
				}
				req.Why = d.Why
				res, err = s.Put(ctx, req)
				if err == nil {
					sum.Distinct++
				}
			}
		}
		if errors.As(err, &ge) {
			c := conflict{Key: r.Key, Title: r.Title}
			for _, d := range ge.Duplicates {
				c.Duplicates = append(c.Duplicates, conflictCandidate{Key: d.Record.Key, Title: d.Record.Title, Similarity: d.Similarity, Shared: d.Shared})
			}
			sum.Conflicts = append(sum.Conflicts, c)
			continue
		}
		var ee *store.ExistsError
		if errors.As(err, &ee) {
			sum.Existing = append(sum.Existing, r.Key)
			continue
		}
		if err != nil {
			sum.Failed = append(sum.Failed, r.Key+": "+err.Error())
			continue
		}
		if res.Created {
			sum.Created++
		}
		if len(res.Related) > 0 {
			var rl []conflictCandidate
			for _, c := range res.Related {
				rl = append(rl, conflictCandidate{Key: c.Record.Key, Title: c.Record.Title, Similarity: c.Similarity, Shared: c.Shared})
			}
			line, _ := json.Marshal(struct {
				Key     string              `json:"key"`
				Related []conflictCandidate `json:"related"`
			}{r.Key, rl})
			relatedBuf.Write(line)
			relatedBuf.WriteByte('\n')
		}
		sum.Superseded = append(sum.Superseded, res.Superseded...)
	}
	if err := s.Optimize(ctx); err != nil {
		return fail(e, err)
	}
	if sum.Created > 0 {
		if err := writeCore(e, s); err != nil {
			return fail(e, err)
		}
	}
	if *relatedPath != "" {
		if err := os.WriteFile(*relatedPath, relatedBuf.Bytes(), 0o600); err != nil {
			return fail(e, err)
		}
	}
	if *conflictsPath != "" {
		var buf bytes.Buffer
		for _, c := range sum.Conflicts {
			line, _ := json.Marshal(c)
			buf.Write(line)
			buf.WriteByte('\n')
		}
		if err := os.WriteFile(*conflictsPath, buf.Bytes(), 0o600); err != nil {
			return fail(e, err)
		}
	}
	for _, c := range sum.Conflicts {
		keys := make([]string, len(c.Duplicates))
		for i, d := range c.Duplicates {
			keys[i] = fmt.Sprintf("%s (%.2f)", d.Key, d.Similarity)
		}
		_, _ = fmt.Fprintf(e.Stderr, "mem import-records: %s looks like a duplicate of %s\n", c.Key, strings.Join(keys, ", "))
	}
	for _, k := range sum.Existing {
		_, _ = fmt.Fprintf(e.Stderr, "mem import-records: record %s already exists\n", k)
	}
	for _, m := range sum.Failed {
		_, _ = fmt.Fprintf(e.Stderr, "mem import-records: %s\n", m)
	}
	if code := printImport(e, f.asJSON(), sum); code != 0 {
		return code
	}
	switch {
	case len(sum.Failed) > 0:
		return 1
	case len(sum.Conflicts) > 0, len(sum.Existing) > 0:
		return 3
	}
	return 0
}

func printImport(e env.Env, asJSON bool, sum importSummary) int {
	if asJSON {
		if err := printJSON(e.Stdout, sum); err != nil {
			return fail(e, err)
		}
		return 0
	}
	if sum.DryRun {
		_, _ = fmt.Fprintf(e.Stdout, "dry run: %d records valid, nothing written\n", sum.Records)
		return 0
	}
	_, _ = fmt.Fprintf(e.Stdout, "%d records: %d created (%d by distinct decision), %d superseded, %d gate conflicts, %d failed\n",
		sum.Records, sum.Created, sum.Distinct, len(sum.Superseded), len(sum.Conflicts), len(sum.Failed))
	return 0
}

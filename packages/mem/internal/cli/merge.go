package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"mem/internal/env"
	"mem/internal/store"
)

type mergeItem struct {
	Key        string              `json:"key"`
	Action     string              `json:"action"`
	Kind       store.Kind          `json:"kind"`
	Title      string              `json:"title,omitempty"`
	Supersedes []string            `json:"supersedes,omitempty"`
	Duplicates []conflictCandidate `json:"duplicates,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type mergeSummary struct {
	From   string      `json:"from"`
	DryRun bool        `json:"dry_run,omitempty"`
	Items  []mergeItem `json:"items"`
}

func parseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Time{}, invalidRecord("--since %q is neither RFC 3339 nor YYYY-MM-DD", s)
}

func sameRecord(a, b store.Record) bool {
	return a.Kind == b.Kind && slices.Equal(a.Scopes, b.Scopes) && a.Core == b.Core && a.Title == b.Title &&
		a.Body == b.Body && a.TTLDays == b.TTLDays && a.VerifyCmd == b.VerifyCmd && slices.Equal(a.Triggers, b.Triggers)
}

func cmdMerge(e env.Env, args []string) int {
	f := newFlags(&e, "merge", "--from DB [--since TIME] [--key K]... [--source S] [--why TEXT] [--dry-run] [--json]", true)
	from := f.String("from", "", "database the records come from, opened read-only")
	since := f.String("since", "", "only records changed after this time, RFC 3339 or YYYY-MM-DD")
	var keys multi
	f.Var(&keys, "key", "only these keys; repeatable, comma-separated")
	source := f.String("source", "", "source stored with every merged record (default merge:<from>)")
	why := f.String("why", "merge", "reason kept in history")
	dry := f.Bool("dry-run", false, "list what would be merged, write nothing")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		return f.usageErr("unexpected arguments " + strings.Join(pos, " "))
	}
	if *from == "" {
		return f.usageErr("missing --from")
	}
	after, err := parseSince(*since)
	if err != nil {
		return fail(e, err)
	}
	src := *source
	if src == "" {
		src = "merge:" + *from
	}
	ctx := context.Background()
	in, err := store.Open(ctx, *from, store.Options{Now: e.Now, ReadOnly: true, Access: store.Access{All: true}})
	if err != nil {
		return fail(e, fmt.Errorf("--from %s: %w", *from, err))
	}
	defer func() { _ = in.Close() }()
	all, err := in.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return fail(e, err)
	}
	replaced := map[string][]string{}
	var picked []store.Record
	for _, r := range all {
		if r.Status == store.Superseded && r.SupersededBy != "" {
			replaced[r.SupersededBy] = append(replaced[r.SupersededBy], r.Key)
		}
		if r.Status != store.Current || r.Kind == store.Log || !r.UpdatedAt.After(after) {
			continue
		}
		if len(keys) > 0 && !slices.Contains(keys, r.Key) {
			continue
		}
		picked = append(picked, r)
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].Key < picked[j].Key })

	s, err := openStore(e, *dry)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	sum := mergeSummary{From: *from, DryRun: *dry}
	written := false
	gate := store.DefaultGate()
	for _, r := range picked {
		it := mergeItem{Key: r.Key, Kind: r.Kind, Title: r.Title}
		old, err := s.Get(ctx, r.Key)
		exists := err == nil
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fail(e, err)
		}
		if exists && old.Status == store.Current && sameRecord(old, r) {
			it.Action = "same"
			sum.Items = append(sum.Items, it)
			continue
		}
		for _, k := range replaced[r.Key] {
			if o, err := s.Get(ctx, k); err == nil && (o.Status == store.Current || o.Status == store.Expired) {
				it.Supersedes = append(it.Supersedes, k)
			}
		}
		core, title, body, triggers, verify := r.Core, r.Title, r.Body, r.Triggers, r.VerifyCmd
		req := store.PutRequest{
			Key: r.Key, Kind: r.Kind, Scopes: r.Scopes, Core: &core, Title: &title, Body: &body,
			Triggers: &triggers, VerifyCmd: &verify, Source: &src, Why: *why, Gate: gate,
		}
		if r.Kind == store.State {
			ttl := r.TTLDays
			req.TTLDays = &ttl
		}
		if exists {
			it.Action = "update"
			req.Update = true
			if len(it.Supersedes) > 0 {
				it.Action, it.Error = "failed", "supersedes "+strings.Join(it.Supersedes, ", ")+" while updating an existing key"
				sum.Items = append(sum.Items, it)
				continue
			}
		} else {
			it.Action = "new"
			req.Supersedes = it.Supersedes
		}
		if *dry {
			if !exists {
				neigh, err := s.Neighbours(ctx, r, it.Supersedes, gate.Neighbours)
				if err != nil {
					return fail(e, err)
				}
				for _, c := range neigh {
					if gate.Duplicate(c) {
						it.Duplicates = append(it.Duplicates, conflictCandidate{Key: c.Record.Key, Title: c.Record.Title, Similarity: c.Similarity, Shared: c.Shared})
					}
				}
				if len(it.Duplicates) > 0 {
					it.Action = "conflict"
				}
			}
			sum.Items = append(sum.Items, it)
			continue
		}
		_, err = s.Put(ctx, req)
		var ge *store.GateError
		switch {
		case errors.As(err, &ge):
			it.Action = "conflict"
			for _, c := range ge.Duplicates {
				it.Duplicates = append(it.Duplicates, conflictCandidate{Key: c.Record.Key, Title: c.Record.Title, Similarity: c.Similarity, Shared: c.Shared})
			}
		case err != nil:
			it.Action, it.Error = "failed", err.Error()
		default:
			written = true
		}
		sum.Items = append(sum.Items, it)
	}
	if written {
		if err := writeCore(e, s); err != nil {
			return fail(e, err)
		}
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, sum); err != nil {
			return fail(e, err)
		}
	} else {
		for _, it := range sum.Items {
			line := it.Action + "\t" + it.Key + "\t[" + string(it.Kind) + "] " + it.Title
			if len(it.Supersedes) > 0 {
				line += "\tsupersedes " + strings.Join(it.Supersedes, ", ")
			}
			if len(it.Duplicates) > 0 {
				d := make([]string, len(it.Duplicates))
				for i, c := range it.Duplicates {
					d[i] = fmt.Sprintf("%s (%.2f)", c.Key, c.Similarity)
				}
				line += "\tduplicate of " + strings.Join(d, ", ")
			}
			if it.Error != "" {
				line += "\t" + it.Error
			}
			_, _ = fmt.Fprintln(e.Stdout, line)
		}
	}
	code = 0
	for _, it := range sum.Items {
		switch it.Action {
		case "failed":
			return 1
		case "conflict":
			code = 3
		}
	}
	return code
}

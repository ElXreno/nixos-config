package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"mem/internal/endb"
	"mem/internal/env"
	"mem/internal/vec"
)

func cmdEmbed(e env.Env, args []string) int {
	fs := flag.NewFlagSet("mem embed", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(e.Stderr, "usage: mem embed --url URL --model M --in DIR --out DIR [--which docs|queries|both] [--query-prefix S] [--doc-prefix S]")
		_, _ = fmt.Fprintln(e.Stderr, "       [--query-suffix S] [--doc-suffix S] [--docs-file FILE] [--batch N] [--par N] [--retries N] [--timeout D]")
		fs.PrintDefaults()
	}
	o := vec.Options{}
	fs.StringVar(&o.URL, "url", "", "server base URL; POST /v1/embeddings is used (TEI and llama-server both serve it)")
	fs.StringVar(&o.Model, "model", "", "model name sent in the request")
	fs.StringVar(&o.QueryPrefix, "query-prefix", "", `text put before every query; \n, \t and \\ are expanded`)
	fs.StringVar(&o.DocPrefix, "doc-prefix", "", `text put before every document; \n, \t and \\ are expanded`)
	fs.StringVar(&o.QuerySuffix, "query-suffix", "", `text put after every query; \n, \t and \\ are expanded`)
	fs.StringVar(&o.DocSuffix, "doc-suffix", "", `text put after every document; \n, \t and \\ are expanded`)
	fs.StringVar(&o.In, "in", "", "directory written by mem eval --export-texts")
	fs.StringVar(&o.Out, "out", "", "directory for docs.f32, docs.keys, queries.f32, queries.ids and meta.json")
	fs.StringVar(&o.Which, "which", "both", "docs, queries or both")
	fs.StringVar(&o.DocsFile, "docs-file", "", "documents from this file instead of IN/docs.jsonl, same format")
	fs.IntVar(&o.Batch, "batch", vec.DefaultBatch, "texts per request")
	fs.IntVar(&o.Par, "par", vec.DefaultPar, "requests in flight")
	fs.IntVar(&o.Retries, "retries", vec.DefaultRetries, "retries of a request after a network error, 429 or 5xx")
	fs.DurationVar(&o.Timeout, "timeout", 10*time.Minute, "timeout of one request")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem embed: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	o.QueryPrefix, o.DocPrefix = vec.ExpandEscapes(o.QueryPrefix), vec.ExpandEscapes(o.DocPrefix)
	o.QuerySuffix, o.DocSuffix = vec.ExpandEscapes(o.QuerySuffix), vec.ExpandEscapes(o.DocSuffix)
	o.In, o.Out, o.DocsFile = expand(e, o.In), expand(e, o.Out), expand(e, o.DocsFile)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	meta, err := vec.Embed(ctx, o, e.Stderr)
	if err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "mem embed:", err)
		if strings.HasPrefix(err.Error(), "--") {
			return 2
		}
		return 1
	}
	for _, p := range []struct {
		name string
		m    *vec.PartMeta
	}{{"docs", meta.Docs}, {"queries", meta.Queries}} {
		if p.m != nil {
			_, _ = fmt.Fprintf(e.Stdout, "%s %d, %.1fs, %.1f texts/s\n", p.name, p.m.Count, p.m.Seconds, p.m.TextsPerSecond)
		}
	}
	_, _ = fmt.Fprintf(e.Stdout, "model %s, dim %d, written to %s\n", meta.Model, meta.Dim, o.Out)
	return 0
}

func expand(e env.Env, p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(e.Home(), rest)
	}
	return p
}

func cmdVecdiff(e env.Env, args []string) int {
	f := newFlags(&e, "vecdiff", "DIR_A DIR_B [--which docs|queries|both]", true)
	which := f.String("which", "both", "docs, queries or both")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) != 2 {
		_, _ = fmt.Fprintln(e.Stderr, "mem vecdiff: two run directories are required")
		return 2
	}
	var parts []vec.Part
	switch *which {
	case "docs":
		parts = []vec.Part{vec.PartDocs}
	case "queries":
		parts = []vec.Part{vec.PartQueries}
	case "both":
		parts = []vec.Part{vec.PartDocs, vec.PartQueries}
	default:
		_, _ = fmt.Fprintf(e.Stderr, "mem vecdiff: --which must be docs, queries or both, not %q\n", *which)
		return 2
	}
	res, err := vec.Diff(expand(e, pos[0]), expand(e, pos[1]), parts)
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, res); err != nil {
			return 1
		}
		return 0
	}
	for _, r := range res {
		_, _ = fmt.Fprintf(e.Stdout, "%s count %d mean %.6f p1 %.6f min %.6f\n", r.Part, r.Count, r.Mean, r.P1, r.Min)
	}
	return 0
}

func cmdBuildEN(e env.Env, args []string) int {
	f := newFlags(&e, "build-en", "--db RU.db --en en.jsonl --out EN.db [--allow-missing] [--force]", true)
	en := f.String("en", "", "translations, lines of {id, key, title_en, body_en, issues}")
	out := f.String("out", "", "English copy of the database to write")
	missing := f.Bool("allow-missing", false, "keep the original text of records without a translation")
	force := f.Bool("force", false, "replace an existing --out")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem build-en: unexpected argument %q\n", pos[0])
		return 2
	}
	if *f.db == "" || *en == "" || *out == "" {
		_, _ = fmt.Fprintln(e.Stderr, "mem build-en: --db, --en and --out are required")
		return 2
	}
	res, err := endb.Build(context.Background(), endb.Options{In: expand(e, *f.db), EN: expand(e, *en), Out: expand(e, *out), AllowMissing: *missing, Force: *force})
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, res); err != nil {
			return 1
		}
		return 0
	}
	_, _ = fmt.Fprintf(e.Stdout, "records %d, replaced %d, missing %d, translations with issues %d, written to %s\n", res.Records, res.Replaced, res.Missing, res.WithIssues, *out)
	return 0
}

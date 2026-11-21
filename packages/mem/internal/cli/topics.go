package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"mem/internal/env"
	"mem/internal/store"
)

func cmdTopics(e env.Env, args []string) int {
	if len(args) == 0 || (args[0] != "set" && args[0] != "list") {
		_, _ = fmt.Fprintln(e.Stderr, "usage: mem topics set --map FILE.tsv [--strict] [--json] | mem topics list [--json]")
		return 2
	}
	if args[0] == "set" {
		return topicsSet(e, args[1:])
	}
	return topicsList(e, args[1:])
}

func parseTopicMap(r io.Reader) ([]store.TopicAssign, error) {
	var out []store.TopicAssign
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("%w: line %d needs key, topic id and topic title separated by tabs", store.ErrInvalid, n)
		}
		out = append(out, store.TopicAssign{Key: strings.TrimSpace(parts[0]), Topic: strings.TrimSpace(parts[1]), Title: parts[2]})
	}
	return out, sc.Err()
}

func topicsSet(e env.Env, args []string) int {
	f := newFlags(&e, "topics set", "--map FILE.tsv [--strict]", true)
	mapFile := f.String("map", "", "TSV file of key, topic id and topic title per line, - for stdin")
	strict := f.Bool("strict", false, "exit 3 when a key of the map is not in the store (the rest is still applied)")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 || *mapFile == "" {
		_, _ = fmt.Fprintln(e.Stderr, "mem topics set: needs --map FILE.tsv and no other arguments")
		return 2
	}
	in := e.Stdin
	if *mapFile != "-" {
		fh, err := os.Open(*mapFile)
		if err != nil {
			return fail(e, err)
		}
		defer func() { _ = fh.Close() }()
		in = fh
	}
	as, err := parseTopicMap(in)
	if err != nil {
		return fail(e, err)
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	res, err := s.SetTopics(context.Background(), as)
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, res); err != nil {
			return fail(e, err)
		}
	} else {
		_, _ = fmt.Fprintf(e.Stdout, "topics %d, titles changed %d, assigned %d, unchanged %d, unknown keys %d\n",
			res.Topics, res.Titles, res.Assigned, res.Unchanged, len(res.Unknown))
	}
	if len(res.Unknown) > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "mem topics set: not in the store: %s\n", strings.Join(res.Unknown, ", "))
		if *strict {
			return 3
		}
	}
	return 0
}

func topicsList(e env.Env, args []string) int {
	f := newFlags(&e, "topics list", "[--json]", true)
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem topics list: unexpected arguments")
		return 2
	}
	s, err := openStore(e, true)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	topics, err := s.Topics(context.Background())
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if topics == nil {
			topics = []store.TopicInfo{}
		}
		return fail(e, printJSON(e.Stdout, topics))
	}
	for _, t := range topics {
		_, _ = fmt.Fprintf(e.Stdout, "%s\t%d\t%d\t%s\n", t.ID, t.Records, t.Logs, t.Title)
	}
	return 0
}

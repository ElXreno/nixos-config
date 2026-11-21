package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"mem/internal/env"
	"mem/internal/store"
)

func cmdEdit(e env.Env, args []string) int {
	f := newFlags(&e, "edit", "KEY --old TEXT --new TEXT [--all] [--why TEXT]\nreplaces one exact occurrence of TEXT in the body; - reads that text from stdin", true)
	f.example = "mem edit KEY --old 'port 8096' --new 'port 8196'"
	oldText := f.String("old", "", "text to replace, exactly as it stands in the body; - reads stdin")
	newText := f.String("new", "", "replacement, may be empty to delete; - reads stdin")
	all := f.Bool("all", false, "replace every occurrence instead of requiring a single one")
	why := f.String("why", "", "reason kept in history")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	switch {
	case len(pos) != 1:
		return f.usageErr("needs exactly one KEY")
	case !f.set("old") || !f.set("new"):
		return f.usageErr("needs --old and --new")
	case *oldText == "-" && *newText == "-":
		return f.usageErr("only one of --old and --new can read stdin")
	}
	if *oldText == "-" || *newText == "-" {
		data, err := readStdin(e.Stdin)
		if errors.Is(err, errNoStdin) {
			return f.usageErr("- " + err.Error())
		} else if err != nil {
			return fail(e, err)
		}
		text := strings.TrimRight(string(data), "\n")
		if *oldText == "-" {
			*oldText = text
		} else {
			*newText = text
		}
	}
	s, err := openStore(e, false)
	if err != nil {
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	res, err := s.Edit(context.Background(), store.EditRequest{Key: pos[0], Old: *oldText, New: *newText, All: *all, Why: *why})
	if err != nil {
		return fail(e, err)
	}
	if f.asJSON() {
		if err := printJSON(e.Stdout, res); err != nil {
			return fail(e, err)
		}
	} else {
		state := "updated"
		if res.Unchanged {
			state = "unchanged"
		}
		_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\tversion %d\n", res.Key, state, res.Version)
	}
	if !res.Unchanged {
		if err := writeCore(e, s); err != nil {
			return fail(e, err)
		}
	}
	return 0
}

type days int

func (d *days) String() string { return strconv.Itoa(int(*d)) }

func (d *days) Set(v string) error {
	n, err := parseDays(v)
	if err != nil {
		return err
	}
	*d = days(n)
	return nil
}

const maxTTLDays = 36500

var errDays = fmt.Errorf("want days as N or Nd, weeks as Nw, or hours as Nh in whole days, from 1 to %d days", maxTTLDays)

func parseDays(v string) (int, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	unit := 1
	hours := false
	switch {
	case strings.HasSuffix(v, "d"):
		v = v[:len(v)-1]
	case strings.HasSuffix(v, "w"):
		v, unit = v[:len(v)-1], 7
	case strings.HasSuffix(v, "h"):
		v, hours = v[:len(v)-1], true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, errDays
	}
	if hours {
		if n%24 != 0 {
			return 0, errDays
		}
		n /= 24
	}
	if n > maxTTLDays/unit {
		return 0, errDays
	}
	return n * unit, nil
}

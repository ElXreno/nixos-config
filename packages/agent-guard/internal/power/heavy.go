package power

import (
	"fmt"
	"path"
	"strings"

	"agent-guard/internal/shell"
)

var idle = []string{"--help", "-h", "--version", "--dry-run", "--no-build"}

type Pattern struct {
	Text      string
	shown     string
	program   string
	subs      []string
	options   []string
	forbidden []string
}

func Parse(texts []string) ([]Pattern, error) {
	out := make([]Pattern, 0, len(texts))
	for _, t := range texts {
		words := strings.Fields(t)
		if len(words) == 0 {
			return nil, fmt.Errorf("heavy command pattern %q is empty", t)
		}
		p := Pattern{Text: strings.Join(words, " "), program: words[0]}
		if strings.HasPrefix(p.program, "!") || strings.HasPrefix(p.program, "-") {
			return nil, fmt.Errorf("heavy command pattern %q must start with the program", t)
		}
		for _, w := range words {
			g := strings.TrimPrefix(w, "!")
			if g == "" {
				return nil, fmt.Errorf("heavy command pattern %q: a lone ! excludes nothing", t)
			}
			if _, err := path.Match(g, ""); err != nil {
				return nil, fmt.Errorf("heavy command pattern %q: %w", t, err)
			}
		}
		for _, w := range words[1:] {
			switch {
			case strings.HasPrefix(w, "!"):
				p.forbidden = append(p.forbidden, w[1:])
			case strings.HasPrefix(w, "-"):
				p.options = append(p.options, w)
			default:
				p.subs = append(p.subs, w)
			}
		}
		p.shown = p.program
		for _, w := range words[1:] {
			if !strings.HasPrefix(w, "!") {
				p.shown += " " + w
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func glob(pattern, word string) bool {
	ok, _ := path.Match(pattern, word)
	return ok
}

func given(pattern, arg string) bool {
	if glob(pattern, arg) {
		return true
	}
	name, value, ok := strings.Cut(arg, "=")
	return ok && value != "false" && strings.HasPrefix(name, "-") && glob(pattern, name)
}

func anyGiven(patterns, args []string) bool {
	for _, p := range patterns {
		for _, a := range args {
			if given(p, a) {
				return true
			}
		}
	}
	return false
}

func option(a string) bool {
	return len(a) > 1 && (a[0] == '-' || a[0] == '+')
}

func (p Pattern) Match(cmd shell.Command) bool {
	if p.program == "" || !glob(p.program, cmd.Head()) {
		return false
	}
	args := cmd.Args()
	for i, a := range args {
		if a == "--" {
			args = args[:i]
			break
		}
	}
	if anyGiven(idle, args) || anyGiven(p.forbidden, args) {
		return false
	}
	for _, o := range p.options {
		if !anyGiven([]string{o}, args) {
			return false
		}
	}
	next, value := 0, false
	for _, a := range args {
		if next == len(p.subs) {
			break
		}
		switch {
		case option(a):
			value = a[0] == '-' && !strings.Contains(a, "=")
		case glob(p.subs[next], a):
			next, value = next+1, false
		case value:
			value = false
		default:
			return false
		}
	}
	return next == len(p.subs)
}

func Heavy(cmds []shell.Command, patterns []Pattern) (Pattern, bool) {
	for _, c := range cmds {
		if c.Kind == shell.Remote {
			continue
		}
		for _, p := range patterns {
			if p.Match(c) {
				return p, true
			}
		}
	}
	return Pattern{}, false
}

// Package replay runs the guards over a corpus of real commands and compares two
// such runs, so a guard change can be judged against history in seconds.
package replay

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"agent-guard/internal/call"
	"agent-guard/internal/checks"
	"agent-guard/internal/guard"
	"agent-guard/internal/text"
	"agent-guard/internal/transcript"
	"agent-guard/internal/verdict"
)

// Corpus writes every distinct shell command found in the transcripts,
// NUL-separated.
func Corpus(src transcript.Source, root string, topOnly bool, w io.Writer) (int, transcript.Stats, error) {
	records, st, err := transcript.Walk(src, root, topOnly)
	if err != nil {
		return 0, st, err
	}
	seen := map[string]struct{}{}
	out := bufio.NewWriter(w)
	for _, r := range records {
		cmd := r.Call.Command
		if r.Call.Kind != call.Shell || cmd == "" || strings.ContainsRune(cmd, 0) {
			continue
		}
		if _, dup := seen[cmd]; dup {
			continue
		}
		seen[cmd] = struct{}{}
		if _, err := io.WriteString(out, cmd+"\x00"); err != nil {
			return len(seen), st, err
		}
	}
	return len(seen), st, out.Flush()
}

func readCorpus(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(data), "\x00")
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func short(cmd string) string {
	return text.Clip(strings.NewReplacer("\n", " ", "\t", " ").Replace(cmd), 220)
}

func hash12(cmd string) string {
	sum := sha256.Sum256([]byte(cmd))
	return hex.EncodeToString(sum[:])[:12]
}

// Options narrow a replay: only one check, or the full commands NUL-separated
// instead of TSV so they can be fed back into explain.
type Options struct {
	Check string
	Full  bool
}

type Stats struct {
	Commands int
	Counts   map[string]int
}

// Run prints one TSV line per verdict: check, level, hash of the command,
// truncated command. A command the guard would rewrite prints its rg-replace
// verdict as rewrite.
func Run(corpus string, e *guard.Engine, opts Options, w io.Writer) (Stats, error) {
	cmds, err := readCorpus(corpus)
	if err != nil {
		return Stats{}, err
	}
	st := Stats{Commands: len(cmds), Counts: map[string]int{}}
	out := bufio.NewWriter(w)
	for _, c := range cmds {
		hit := false
		d := e.PreTool(call.Call{Kind: call.Shell, Command: c})
		for _, v := range d.Verdicts {
			if opts.Check != "" && v.Check != opts.Check {
				continue
			}
			level := v.Level.String()
			if d.Outcome == verdict.Rewrite && v.Check == checks.RgReplace {
				level = string(verdict.Rewrite)
			}
			st.Counts[v.Check+"\t"+level]++
			hit = true
			if !opts.Full {
				if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", v.Check, level, hash12(c), short(c)); err != nil {
					return st, err
				}
			}
		}
		if hit && opts.Full {
			if _, err := fmt.Fprintf(out, "%s\x00", c); err != nil {
				return st, err
			}
		}
	}
	return st, out.Flush()
}

type hit struct {
	check, level, hash, text string
}

func readHits(path string) (hits []hit, hashed bool, err error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if cerr := fh.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	hashed = true
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "\t")
		var h hit
		switch {
		case len(parts) == 4 && isHash(parts[2]):
			h = hit{parts[0], parts[1], parts[2], parts[3]}
		case len(parts) >= 3:
			h = hit{check: parts[0], level: parts[1], text: strings.Join(parts[2:], "\t")}
			hashed = false
		default:
			continue
		}
		if l, err := verdict.ParseLevel(h.level); err == nil {
			h.level = l.String()
		}
		hits = append(hits, h)
	}
	if err := sc.Err(); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	return hits, hashed, nil
}

func isHash(s string) bool {
	if len(s) != 12 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

type outcomes struct {
	levels map[string]map[string]struct{}
	text   map[string]string
}

func index(hits []hit, byHash bool) outcomes {
	o := outcomes{levels: map[string]map[string]struct{}{}, text: map[string]string{}}
	for _, h := range hits {
		key := h.check + "\t" + h.text
		if byHash {
			key = h.check + "\t" + h.hash
		}
		if o.levels[key] == nil {
			o.levels[key] = map[string]struct{}{}
		}
		o.levels[key][h.level] = struct{}{}
		o.text[key] = h.check + "\t" + h.text
	}
	return o
}

// Diff compares two replay outputs. A command that tripped before and is silent
// now is a regression; the rest is reported for review. Commands are matched by
// hash when both files carry one, by their truncated text otherwise.
func Diff(oldPath, newPath string, w io.Writer) error {
	oldHits, oldHashed, err := readHits(oldPath)
	if err != nil {
		return err
	}
	newHits, newHashed, err := readHits(newPath)
	if err != nil {
		return err
	}
	byHash := oldHashed && newHashed
	before, after := index(oldHits, byHash), index(newHits, byHash)
	var regressions, caught []string
	changes := map[string]int{}
	for k, o := range before.levels {
		n, ok := after.levels[k]
		if !ok {
			regressions = append(regressions, before.text[k])
			continue
		}
		if a, b := levels(o), levels(n); a != b {
			changes[strings.SplitN(k, "\t", 2)[0]+"\t"+a+" -> "+b]++
		}
	}
	for k := range after.levels {
		if _, ok := before.levels[k]; !ok {
			caught = append(caught, after.text[k])
		}
	}
	sort.Strings(regressions)
	sort.Strings(caught)
	perCheck := map[string]int{}
	for _, c := range caught {
		perCheck[strings.SplitN(c, "\t", 2)[0]]++
	}

	out := bufio.NewWriter(w)
	line := func(format string, a ...any) {
		_, _ = fmt.Fprintf(out, format, a...)
	}
	line("== regressions: tripped before, silent now (%d) ==\n", len(regressions))
	for _, r := range regressions {
		line("  %s\n", r)
	}
	line("\n== newly caught (%d) ==\n", len(caught))
	for _, k := range sortedKeys(perCheck) {
		line("%6d  %s\n", perCheck[k], k)
	}
	for _, c := range caught {
		line("  %s\n", c)
	}
	line("\n== outcome changes ==\n")
	for _, k := range sortedKeys(changes) {
		line("%6d  %s\n", changes[k], k)
	}
	return out.Flush()
}

func levels(m map[string]struct{}) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, "+")
}

func sortedKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

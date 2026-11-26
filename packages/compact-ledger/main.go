package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const instructions = `Besides the usual sections, keep a section named "Ruled out": every approach, hypothesis, fix or command that was tried and failed, did not help or was rejected, one line each with the reason. Carry over every item from earlier "Ruled out" lists and from the earlier compaction summaries injected into this conversation, then add the new ones; never drop an item. Keep a section named "Established facts" with verified findings that later work relies on, carried over the same way. Before writing "Current Work" and "Optional Next Step", check the next step against "Ruled out" and name one that has not been tried.`

const header = `Earlier compaction summaries of this session, oldest first (%d of %d). The latest summary may have lost details that these keep: before planning the next step, check what they record as done, ruled out or failed, and do not redo it.`

var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type hookInput struct {
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name"`
	Source         string `json:"source"`
	Trigger        string `json:"trigger"`
	CompactSummary string `json:"compact_summary"`
}

type entry struct {
	Time    time.Time `json:"time"`
	Trigger string    `json:"trigger"`
	Summary string    `json:"summary"`
}

type ledger struct {
	dir    string
	budget int
	now    func() time.Time
}

func (l ledger) path(session string) (string, error) {
	if !sessionPattern.MatchString(session) {
		return "", fmt.Errorf("invalid session id %q", session)
	}
	return filepath.Join(l.dir, session+".jsonl"), nil
}

func (l ledger) handle(in hookInput, out io.Writer) error {
	switch in.HookEventName {
	case "PreCompact":
		_, err := fmt.Fprintln(out, instructions)
		return err
	case "PostCompact":
		return l.record(in)
	case "SessionStart":
		if in.Source != "compact" {
			return nil
		}
		return l.inject(in.SessionID, out)
	}
	return nil
}

func (l ledger) record(in hookInput) error {
	if strings.TrimSpace(in.CompactSummary) == "" {
		return nil
	}
	path, err := l.path(in.SessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	line, err := json.Marshal(entry{Time: l.now().UTC(), Trigger: in.Trigger, Summary: in.CompactSummary})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	return errors.Join(werr, f.Close())
}

func (l ledger) load(session string) ([]entry, error) {
	path, err := l.path(session)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var entries []entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for scanner.Scan() {
		var e entry
		if json.Unmarshal(scanner.Bytes(), &e) == nil && strings.TrimSpace(e.Summary) != "" {
			entries = append(entries, e)
		}
	}
	return entries, scanner.Err()
}

func (l ledger) inject(session string, out io.Writer) error {
	entries, err := l.load(session)
	if err != nil || len(entries) == 0 {
		return err
	}
	first, used := len(entries), 0
	for first > 0 {
		size := len(entries[first-1].Summary)
		if used+size > l.budget && first < len(entries) {
			break
		}
		used += size
		first--
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<earlier-compaction-summaries>\n"+header+"\n", len(entries)-first, len(entries))
	for i := first; i < len(entries); i++ {
		e := entries[i]
		summary := e.Summary
		if len(summary) > l.budget {
			summary = summary[:l.budget]
		}
		fmt.Fprintf(&b, "\n## Compaction %d of %d (%s, %s)\n%s\n", i+1, len(entries), e.Time.Format("2006-01-02 15:04 UTC"), e.Trigger, strings.TrimSpace(summary))
	}
	b.WriteString("</earlier-compaction-summaries>\n")
	_, err = io.WriteString(out, b.String())
	return err
}

func defaultDir() string {
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "compact-ledger")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "compact-ledger")
	}
	return filepath.Join(home, ".local", "state", "compact-ledger")
}

func main() {
	flags := flag.NewFlagSet("compact-ledger", flag.ExitOnError)
	dir := flags.String("dir", defaultDir(), "directory of the per-session ledgers")
	budget := flags.Int("budget", 120000, "characters of earlier summaries injected after a compaction")
	if len(os.Args) < 2 || os.Args[1] != "hook" {
		fmt.Fprintln(os.Stderr, "usage: compact-ledger hook [-dir DIR] [-budget CHARS] < hook-input.json")
		os.Exit(2)
	}
	_ = flags.Parse(os.Args[2:])
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "compact-ledger:", err)
		os.Exit(1)
	}
	l := ledger{dir: *dir, budget: *budget, now: time.Now}
	if err := l.handle(in, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "compact-ledger:", err)
		os.Exit(1)
	}
}

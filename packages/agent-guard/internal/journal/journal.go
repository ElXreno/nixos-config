// Package journal keeps one JSON line per hook decision, so what the guard
// asked, denied or let through can be counted without digging in transcripts.
package journal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-guard/internal/call"
	"agent-guard/internal/harness"
	"agent-guard/internal/verdict"
)

const (
	PreBash           = "pre-bash"
	PreEdit           = "pre-edit"
	PermissionRequest = "permission-request"
	AgentMemoryWrite  = "agent-memory-write"
	DialogResult      = "dialog-result"
	PostTool          = "post-tool"
	UserPrompt        = "user-prompt"
	Evidence          = "evidence"
)

const maxLine = 1 << 20

type Fired struct {
	Check string        `json:"check"`
	Level verdict.Level `json:"level"`
	Fixed bool          `json:"fixed,omitempty"`
}

type Entry struct {
	Time      time.Time       `json:"time"`
	Harness   harness.ID      `json:"harness,omitempty"`
	Session   string          `json:"session,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Hook      string          `json:"hook"`
	Tool      string          `json:"tool,omitempty"`
	Kind      call.Kind       `json:"kind,omitempty"`
	Level     verdict.Outcome `json:"level"`
	Checks    []Fired         `json:"checks,omitempty"`
	Input     string          `json:"input,omitempty"`
	Notes     []string        `json:"notes,omitempty"`
}

// UnmarshalJSON also reads the first journal format, where checks was a list
// of names that all took the entry's level.
func (e *Entry) UnmarshalJSON(b []byte) error {
	type plain Entry
	var raw struct {
		plain
		Level  string            `json:"level"`
		Checks []json.RawMessage `json:"checks"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*e = Entry(raw.plain)
	e.Level = verdict.Outcome(raw.Level)
	if l, err := verdict.ParseLevel(raw.Level); err == nil {
		e.Level = l.Outcome()
	}
	e.Checks = nil
	for _, c := range raw.Checks {
		var name string
		if json.Unmarshal(c, &name) == nil {
			e.Checks = append(e.Checks, legacyFired(name, e.Level))
			continue
		}
		var f Fired
		if err := json.Unmarshal(c, &f); err != nil {
			return err
		}
		e.Checks = append(e.Checks, f)
	}
	return nil
}

func legacyFired(name string, outcome verdict.Outcome) Fired {
	if outcome == verdict.Rewrite {
		return Fired{Check: name, Level: verdict.Deny, Fixed: true}
	}
	l, _ := verdict.ParseLevel(string(outcome))
	return Fired{Check: name, Level: l}
}

// Append adds e to the journal at path. It never fails the hook: a journal
// that cannot be written is simply skipped.
func Append(path string, e Entry) {
	if path == "" {
		return
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
}

// Read returns every entry of the journal and the number of lines it skipped
// because they did not decode or ran over the line limit.
func Read(path string) ([]Entry, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	var out []Entry
	skipped := 0
	r := bufio.NewReaderSize(f, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var e Entry
			if len(line) > maxLine || json.Unmarshal(line, &e) != nil {
				skipped++
			} else {
				out = append(out, e)
			}
		}
		if errors.Is(err, io.EOF) {
			return out, skipped, nil
		}
		if err != nil {
			return out, skipped, err
		}
	}
}

type Count struct {
	Key   string
	Count int
}

type Summary struct {
	Calls   map[string]int
	Verdict []Count
	Dialogs []Count
	Skipped int
}

// call identifies a tool call across hooks. A PermissionRequest may carry no
// tool_use_id, so the session, tool and input stand in for it.
func (e Entry) call() string { return e.Session + "\x00" + e.Tool + "\x00" + e.Input }

func (e Entry) keys() []string {
	if e.ToolUseID == "" {
		return []string{e.call()}
	}
	return []string{"id\x00" + e.ToolUseID, e.call()}
}

func (f Fired) key() string {
	if f.Fixed {
		return string(verdict.Rewrite) + " " + f.Check
	}
	return f.Level.String() + " " + f.Check
}

func label(id harness.ID) string {
	if spec, err := harness.ByID(string(id)); err == nil {
		return spec.Label
	}
	return string(id)
}

// Summarize counts the decisions since a moment. A dialog is a
// permission-request the guard let through to the user; it is attributed to
// the checks that asked for the same tool call, or to the harness itself when
// none did. Entries without a harness belong to fallback.
func Summarize(entries []Entry, since time.Time, fallback harness.ID) Summary {
	asked := map[string][]string{}
	for _, e := range entries {
		if e.Level != verdict.Ask.Outcome() || e.Hook == PermissionRequest {
			continue
		}
		var names []string
		for _, f := range e.Checks {
			if f.Level == verdict.Ask {
				names = append(names, f.Check)
			}
		}
		for _, k := range e.keys() {
			asked[k] = names
		}
	}
	calls := map[string]int{}
	perCheck := map[string]int{}
	dialogs := map[string]int{}
	for _, e := range entries {
		if e.Time.Before(since) {
			continue
		}
		calls[e.Hook]++
		if e.Hook == PermissionRequest {
			var names []string
			for _, k := range e.keys() {
				if names = asked[k]; names != nil {
					break
				}
			}
			h := e.Harness
			if h == "" {
				h = fallback
			}
			switch {
			case e.Level == verdict.Deny.Outcome():
				dialogs["auto-denied: "+e.Tool]++
			case names != nil:
				dialogs["guard ask: "+strings.Join(names, ",")]++
			default:
				dialogs[label(h)+" itself: "+e.Tool]++
			}
			continue
		}
		for _, f := range e.Checks {
			perCheck[f.key()]++
		}
	}
	return Summary{Calls: calls, Verdict: ranked(perCheck), Dialogs: ranked(dialogs)}
}

func ranked(m map[string]int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		return out[a].Key < out[b].Key
	})
	return out
}

func (s Summary) Write(w io.Writer) error {
	var b strings.Builder
	var hooks []string
	for h := range s.Calls {
		hooks = append(hooks, h)
	}
	sort.Strings(hooks)
	b.WriteString("calls:\n")
	for _, h := range hooks {
		fmt.Fprintf(&b, "  %6d  %s\n", s.Calls[h], h)
	}
	b.WriteString("dialogs:\n")
	for _, c := range s.Dialogs {
		fmt.Fprintf(&b, "  %6d  %s\n", c.Count, c.Key)
	}
	b.WriteString("verdicts:\n")
	for _, c := range s.Verdict {
		fmt.Fprintf(&b, "  %6d  %s\n", c.Count, c.Key)
	}
	if s.Skipped > 0 {
		fmt.Fprintf(&b, "skipped: %d unreadable journal lines\n", s.Skipped)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

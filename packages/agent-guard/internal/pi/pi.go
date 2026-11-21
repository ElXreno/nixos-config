package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"agent-guard/internal/call"
	"agent-guard/internal/guard"
	"agent-guard/internal/harness"
	"agent-guard/internal/verdict"
)

const (
	ToolCall     = "tool_call"
	DialogResult = "dialog_result"
	ToolResult   = "tool_result"
	Prompt       = "prompt"
)

var Tools = []string{"bash", "process", "edit", "write"}

type Payload struct {
	Event      string          `json:"event"`
	ToolName   string          `json:"toolName"`
	ToolCallID string          `json:"toolCallId"`
	Input      json.RawMessage `json:"input"`
	Cwd        string          `json:"cwd"`
	SessionID  string          `json:"sessionId"`
	HasUI      bool            `json:"hasUI"`
	Approved   bool            `json:"approved"`
}

func Read(r io.Reader) (Payload, error) {
	var p Payload
	err := json.NewDecoder(r).Decode(&p)
	return p, err
}

func (p Payload) envelope() call.Call {
	return call.Call{Harness: harness.Pi, Event: p.Event, Tool: p.ToolName, Session: p.SessionID, CallID: p.ToolCallID, Cwd: p.Cwd}
}

type editPair struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type toolInput struct {
	Command string     `json:"command"`
	Action  string     `json:"action"`
	Cwd     string     `json:"cwd"`
	Path    string     `json:"path"`
	Content string     `json:"content"`
	Edits   []editPair `json:"edits"`
	OldText *string    `json:"oldText"`
	NewText *string    `json:"newText"`
}

func Decode(p Payload, home string) (c call.Call, guarded bool, err error) {
	c = p.envelope()
	raw := bytes.TrimSpace(p.Input)
	if len(raw) == 0 || raw[0] != '{' {
		return c, false, errors.New("input is not an object")
	}
	known := false
	for _, t := range Tools {
		known = known || t == p.ToolName
	}
	if !known {
		return c, false, nil
	}
	var in toolInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return c, false, fmt.Errorf("input of %s: %w", p.ToolName, err)
	}
	switch p.ToolName {
	case "bash":
		c.Kind, c.Command = call.Shell, in.Command
	case "process":
		if in.Action != "start" {
			return c, false, nil
		}
		c.Kind, c.Command = call.Shell, in.Command
		if in.Cwd != "" {
			c.Cwd = join(p.Cwd, in.Cwd)
		}
	case "edit":
		c.Kind = call.Edit
		c.Change = call.Change{Path: Resolve(in.Path, p.Cwd, home), Apply: call.Original, Pairs: make([]call.Pair, 0, len(in.Edits)+1)}
		for _, e := range in.Edits {
			c.Change.Pairs = append(c.Change.Pairs, call.Pair{Old: e.OldText, New: e.NewText})
		}
		if in.OldText != nil && in.NewText != nil {
			c.Change.Pairs = append(c.Change.Pairs, call.Pair{Old: *in.OldText, New: *in.NewText})
		}
	case "write":
		c.Kind = call.Write
		c.Change = call.Change{Path: Resolve(in.Path, p.Cwd, home), Write: true, Content: in.Content}
	}
	return c, true, nil
}

func Envelope(p Payload) call.Call { return p.envelope() }

func join(cwd, p string) string {
	if filepath.IsAbs(p) || cwd == "" {
		return filepath.Clean(p)
	}
	return filepath.Join(cwd, p)
}

var unicodeSpaces = regexp.MustCompile("[\u00A0\u2000-\u200A\u202F\u205F\u3000]")

func Resolve(path, cwd, home string) string {
	p := unicodeSpaces.ReplaceAllString(path, " ")
	p = strings.TrimPrefix(p, "@")
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[2:])
	case strings.HasPrefix(p, "file://"):
		u, err := url.Parse(p)
		if err != nil || (u.Host != "" && u.Host != "localhost") {
			return ""
		}
		p = u.Path
	}
	if strings.HasPrefix(cwd, "~/") {
		cwd = filepath.Join(home, cwd[2:])
	}
	return join(cwd, p)
}

type Confirm struct {
	Message  string `json:"message"`
	Declined string `json:"declined"`
}

type Patch struct {
	Command string `json:"command"`
}

type Output struct {
	Block   string   `json:"block,omitempty"`
	Confirm *Confirm `json:"confirm,omitempty"`
	Patch   *Patch   `json:"patch,omitempty"`
	Append  string   `json:"append,omitempty"`
}

const prefix = "agent-guard: "

func appended(s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

func withPower(s string, d guard.Decision) string {
	if n := d.Power(); n != "" {
		return s + "\n\n" + n
	}
	return s
}

func Encode(d guard.Decision) Output {
	switch d.Outcome {
	case verdict.Deny.Outcome():
		return Output{Block: d.Denial()}
	case verdict.Rewrite:
		return Output{Patch: &Patch{Command: d.Command}, Append: appended(d.Context())}
	case verdict.Ask.Outcome():
		return Output{Confirm: &Confirm{Message: d.Reason(), Declined: withPower("The user declined: "+d.Reason(), d)}, Append: appended(d.Context())}
	case verdict.Advise.Outcome():
		return Output{Append: appended(d.Context())}
	}
	return Output{}
}

func NoUI(d guard.Decision) Output {
	return Output{Block: withPower(d.Reason()+" This needs the user's confirmation, and nobody can answer in this mode.", d)}
}

func Write(w io.Writer, out Output) int {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
	return 0
}

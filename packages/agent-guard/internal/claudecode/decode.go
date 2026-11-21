package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"agent-guard/internal/call"
	"agent-guard/internal/harness"
)

type editPair struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

type editInput struct {
	FilePath   string     `json:"file_path"`
	OldString  string     `json:"old_string"`
	NewString  string     `json:"new_string"`
	ReplaceAll bool       `json:"replace_all"`
	Content    string     `json:"content"`
	Edits      []editPair `json:"edits"`
}

func Decode(in Input) (call.Call, error) {
	c, err := DecodeTool(in.ToolName, in.ToolInput)
	c.Harness = harness.ClaudeCode
	c.Event = in.HookEventName
	c.Session = in.SessionID
	c.Agent = in.AgentID
	c.CallID = in.ToolUseID
	c.Cwd = in.Cwd
	return c, err
}

func DecodeTool(name string, raw json.RawMessage) (call.Call, error) {
	c := call.Call{Tool: name}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = []byte("{}")
	}
	if raw[0] != '{' {
		return c, errors.New("tool_input is not an object")
	}
	switch name {
	case "Bash":
		var b struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(raw, &b); err != nil {
			return c, fmt.Errorf("tool_input of %s: %w", name, err)
		}
		c.Kind, c.Command = call.Shell, b.Command
	case "Edit", "MultiEdit", "Write":
		var e editInput
		if err := json.Unmarshal(raw, &e); err != nil {
			return c, fmt.Errorf("tool_input of %s: %w", name, err)
		}
		c.Change.Path = e.FilePath
		switch name {
		case "Write":
			c.Kind = call.Write
			c.Change.Write, c.Change.Content = true, e.Content
		case "Edit":
			c.Kind = call.Edit
			c.Change.Pairs = []call.Pair{{Old: e.OldString, New: e.NewString, All: e.ReplaceAll}}
		default:
			c.Kind = call.Edit
			c.Change.Pairs = make([]call.Pair, 0, len(e.Edits))
			for _, p := range e.Edits {
				c.Change.Pairs = append(c.Change.Pairs, call.Pair{Old: p.OldString, New: p.NewString, All: p.ReplaceAll})
			}
		}
	default:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return c, fmt.Errorf("tool_input of %s: %w", name, err)
		}
		str := func(keys ...string) string {
			for _, k := range keys {
				var s string
				if json.Unmarshal(fields[k], &s) == nil && s != "" {
					return s
				}
			}
			return ""
		}
		c.Command = str("command")
		c.Change.Path = str("file_path", "path")
		c.URL = str("url")
	}
	return c, nil
}

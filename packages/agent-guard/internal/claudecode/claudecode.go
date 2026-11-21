package claudecode

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"agent-guard/internal/guard"
	"agent-guard/internal/verdict"
)

type Input struct {
	SessionID      string          `json:"session_id"`
	AgentID        string          `json:"agent_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
}

type BashInput struct {
	Command string `json:"command"`
}

func Read(r io.Reader) (Input, error) {
	var in Input
	dec := json.NewDecoder(r)
	if err := dec.Decode(&in); err != nil {
		return in, err
	}
	return in, nil
}

type Output struct {
	Stdout string
	Stderr string
	Code   int
}

func Encode(d guard.Decision, toolInput json.RawMessage) Output {
	switch d.Outcome {
	case verdict.Deny.Outcome():
		return Output{Stderr: d.Denial(), Code: 2}
	case verdict.Rewrite:
		var fields map[string]any
		if json.Unmarshal(toolInput, &fields) != nil || fields == nil {
			return Output{Stderr: d.Denial(), Code: 2}
		}
		fields["command"] = d.Command
		return Output{Stdout: encode(map[string]any{
			"hookEventName":     "PreToolUse",
			"updatedInput":      fields,
			"additionalContext": d.Context(),
		})}
	case verdict.Ask.Outcome():
		spec := map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "ask",
			"permissionDecisionReason": d.Reason(),
		}
		if ctx := d.Context(); ctx != "" {
			spec["additionalContext"] = ctx
		}
		return Output{Stdout: encode(spec)}
	case verdict.Advise.Outcome():
		return Output{Stdout: encode(map[string]any{
			"hookEventName":     "PreToolUse",
			"additionalContext": d.Context(),
		})}
	}
	return Output{}
}

func Context(event string, d guard.Decision) Output {
	ctx := d.Context()
	if ctx == "" {
		return Output{}
	}
	return Output{Stdout: encode(map[string]any{
		"hookEventName":     event,
		"additionalContext": ctx,
	})}
}

func Write(stdout, stderr io.Writer, out Output) int {
	if out.Stdout != "" {
		_, _ = fmt.Fprintln(stdout, out.Stdout)
	}
	if out.Stderr != "" {
		_, _ = fmt.Fprintln(stderr, out.Stderr)
	}
	return out.Code
}

func encode(spec map[string]any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"hookSpecificOutput": spec})
	return strings.TrimSpace(b.String())
}

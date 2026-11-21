package hook

import (
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/env"
	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	defaultHarness = "claude-code"
	maxContextRune = 256
)

func toolPreview(in Input) string {
	fields := inputFields(in.ToolInput)
	lead := field(fields, "command", "file_path", "notebook_path", "path", "url", "pattern", "query", "prompt", "subagent_type")
	desc := field(fields, "description")
	switch {
	case lead != "" && desc != "" && desc != lead:
		lead += " | " + desc
	case lead == "":
		lead = desc
	}
	p, _ := statefile.PreviewN(lead, statefile.InputRunes)
	return p
}

func homeRel(e env.Env, p string) string {
	if p == "" {
		return ""
	}
	if home := filepath.Clean(e.Home()); home != "." && home != string(filepath.Separator) {
		if p == home {
			return "~"
		}
		if rel, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
			return "~/" + rel
		}
	}
	return p
}

func stampTurn(t *statefile.Turn, e env.Env, cfg config, in Input) {
	if t.AgentType == "" {
		t.AgentType = clip(in.AgentType, maxToolName)
	}
	t.Harness = clip(cfg.harness, maxToolName)
	t.Host = clip(e.Host(), maxToolName)
	t.Project = clip(project(e, in), maxContextRune)
	t.Cwd = clip(homeRel(e, cwdOf(e, in)), maxContextRune)
}

func errorTurn(event string, e env.Env, in Input) *statefile.Turn {
	t := &statefile.Turn{Time: e.Now().UTC().Format(time.RFC3339), Session: in.SessionID, Agent: in.AgentID}
	switch event {
	case SessionStart:
		t.Event, t.Source = statefile.EventSession, in.Source
	case PreToolUse:
		t.Event, t.Tool, t.Input = statefile.EventTool, clip(in.ToolName, maxToolName), toolPreview(in)
	default:
		prompt := strings.TrimSpace(in.Prompt)
		if prompt == "" {
			return nil
		}
		preview, cut := statefile.PreviewOf(prompt)
		t.Event, t.PromptSHA, t.Prompt, t.PromptCut, t.PromptChars = statefile.EventPrompt, promptSHA(prompt), preview, cut, utf8.RuneCountInString(prompt)
	}
	return t
}

func firedOf(hits []store.TriggerHit, keys []string) []statefile.Fired {
	var out []statefile.Fired
	for _, k := range keys {
		for _, h := range hits {
			if h.Record.Key == k {
				out = append(out, statefile.Fired{Key: k, Tool: clip(h.Trigger.Tool, maxContextRune), Regex: clip(h.Trigger.Regex, maxContextRune)})
				break
			}
		}
	}
	return out
}

package pi

import (
	"encoding/json"
	"path/filepath"
	"sort"

	"agent-guard/internal/harness"
	"agent-guard/internal/transcript"
)

type Source struct {
	Home string
}

func (Source) Files(root string, _ bool) ([]transcript.File, error) {
	paths, err := filepath.Glob(filepath.Join(root, "--*--", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]transcript.File, 0, len(paths))
	for _, p := range paths {
		out = append(out, transcript.File{Path: p})
	}
	return out, nil
}

type sessionLine struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Cwd     string `json:"cwd"`
	Message *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type toolCallBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s Source) Read(f transcript.File, emit func(transcript.Record)) (int, error) {
	skipped := 0
	var cwd, session string
	err := transcript.EachLine(f.Path, func(line []byte) {
		var rec sessionLine
		if json.Unmarshal(line, &rec) != nil {
			skipped++
			return
		}
		if rec.Type == "session" {
			cwd, session = rec.Cwd, rec.ID
			return
		}
		if rec.Type != "message" || rec.Message == nil || rec.Message.Role != "assistant" {
			return
		}
		var blocks []toolCallBlock
		if json.Unmarshal(rec.Message.Content, &blocks) != nil {
			return
		}
		for _, b := range blocks {
			if b.Type != "toolCall" {
				continue
			}
			p := Payload{Event: ToolCall, ToolName: b.Name, ToolCallID: b.ID, Input: b.Arguments, Cwd: cwd, SessionID: session}
			c, guarded, err := Decode(p, s.Home)
			if err != nil || !guarded {
				continue
			}
			emit(transcript.Record{Harness: harness.Pi, Call: c, Key: b.Name + "\x00" + string(b.Arguments)})
		}
	})
	return skipped, err
}

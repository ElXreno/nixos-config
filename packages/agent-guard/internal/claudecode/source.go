package claudecode

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"agent-guard/internal/harness"
	"agent-guard/internal/transcript"
)

type Source struct{}

func (Source) Files(root string, topOnly bool) ([]transcript.File, error) {
	top, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(top)
	var out []transcript.File
	for _, p := range top {
		out = append(out, transcript.File{Path: p})
	}
	if topOnly {
		return out, nil
	}
	dirs, err := filepath.Glob(filepath.Join(root, "*", "*", "subagents"))
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		_ = filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasPrefix(e.Name(), "agent-") && strings.HasSuffix(e.Name(), ".jsonl") {
				out = append(out, transcript.File{Path: p, Sub: true})
			}
			return nil
		})
	}
	return out, nil
}

type sourceLine struct {
	Cwd       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	Message   *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type toolUse struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (Source) Read(f transcript.File, emit func(transcript.Record)) (int, error) {
	skipped := 0
	err := transcript.EachLine(f.Path, func(line []byte) {
		var rec sourceLine
		if json.Unmarshal(line, &rec) != nil {
			skipped++
			return
		}
		if rec.Message == nil {
			return
		}
		var blocks []toolUse
		if json.Unmarshal(rec.Message.Content, &blocks) != nil {
			return
		}
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			c, err := DecodeTool(b.Name, b.Input)
			if err != nil {
				continue
			}
			c.Harness, c.Session, c.CallID, c.Cwd = harness.ClaudeCode, rec.SessionID, b.ID, rec.Cwd
			emit(transcript.Record{Harness: harness.ClaudeCode, Sub: f.Sub, Call: c, Key: b.Name + "\x00" + string(b.Input)})
		}
	})
	return skipped, err
}

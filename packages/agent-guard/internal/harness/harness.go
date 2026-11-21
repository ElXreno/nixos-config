package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type ID string

const (
	ClaudeCode ID = "claude-code"
	Pi         ID = "pi"
)

type Vocabulary struct {
	Background  string
	Detach      string
	StopTracked string
	ReadFile    string
	WriteTool   string
	EditTool    string
}

type Spec struct {
	ID         ID
	Label      string
	Vocab      Vocabulary
	SessionTag func(session string) string
	TaskDir    func() string
}

var specs = []Spec{
	{
		ID:    ClaudeCode,
		Label: "Claude Code",
		Vocab: Vocabulary{
			Background:  "Pass run_in_background: true to the Bash tool instead: the harness tracks the job across turns, wakes you when it exits, and TaskOutput retrieves its output, so you never need to sleep and poll for it.",
			Detach:      "Pass run_in_background: true to the Bash tool instead: the harness keeps the job alive across turns, wakes you when it exits, and TaskOutput retrieves its output.",
			StopTracked: "If the target is a background task you started through the Bash tool, stop it with TaskStop and its task id.",
			ReadFile:    "should be opened with Read: Edit refuses files not read through it, Read pages large files with offset and limit, and its line numbers anchor citations.",
			WriteTool:   "Write",
			EditTool:    "Edit",
		},
		SessionTag: head4,
		TaskDir: func() string {
			return filepath.Join(os.TempDir(), "claude-"+strconv.Itoa(os.Getuid()))
		},
	},
	{
		ID:    Pi,
		Label: "pi",
		Vocab: Vocabulary{
			Background:  "Start long-running commands with the process tool (action \"start\") instead: it keeps them running, wakes you when they exit or a watched pattern appears in the log, and returns the output on request.",
			Detach:      "Start long-running commands with the process tool (action \"start\") instead: it keeps them alive, wakes you when they exit, and returns the output on request.",
			StopTracked: "If the target is a process you started with the process tool, stop it there (action \"stop\") with its id.",
			ReadFile:    "is better opened with the read tool: it pages large files with offset and limit, and its line numbers anchor citations.",
			WriteTool:   "write",
			EditTool:    "edit",
		},
		SessionTag: tail4,
		TaskDir:    func() string { return "" },
	},
}

func All() []Spec { return append([]Spec(nil), specs...) }

func ByID(id string) (Spec, error) {
	if id == "" {
		id = string(ClaudeCode)
	}
	for _, s := range specs {
		if string(s.ID) == id {
			return s, nil
		}
	}
	return Spec{}, fmt.Errorf("unknown harness %q", id)
}

func head4(s string) string {
	if len(s) > 4 {
		return s[:4]
	}
	return s
}

func tail4(s string) string {
	if len(s) > 4 {
		return s[len(s)-4:]
	}
	return s
}

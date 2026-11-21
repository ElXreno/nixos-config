package checks

import (
	"strings"
	"testing"

	"agent-guard/internal/harness"
	"agent-guard/internal/shell"
)

var claudeOnly = []string{"run_in_background", "TaskOutput", "TaskStop", "Bash tool", "Edit refuses", "opened with Read"}

var aiTypography = []string{"—", "–", "…"}

func TestHarnessAdvice(t *testing.T) {
	cases := []struct {
		check, command, claude, pi string
	}{
		{"background", "server &", "run_in_background", "process tool"},
		{"background", "nohup server", "run_in_background", "process tool"},
		{"background", "setsid server", "run_in_background", "process tool"},
		{"pkill", "pkill -f server", "TaskStop", "process tool"},
		{"shell-tool", "cat notes.txt", "Edit refuses", "read tool"},
	}
	for _, spec := range harness.All() {
		if spec.ID != harness.ClaudeCode && spec.ID != harness.Pi {
			t.Fatalf("harness %s has no expectation in this test", spec.ID)
		}
		cfg := &Settings{Spec: spec}
		for _, c := range cases {
			got := runOne(c.check, shell.Analyze(c.command), cfg)
			ref := runOne(c.check, shell.Analyze(c.command), &Settings{})
			if len(got) != 1 || len(ref) != 1 {
				t.Fatalf("%s %q: want one verdict, got %d and %d", spec.ID, c.command, len(got), len(ref))
			}
			msg := got[0].Message
			expect := c.pi
			if spec.ID == harness.ClaudeCode {
				expect = c.claude
			}
			if !strings.Contains(msg, expect) {
				t.Errorf("%s %q: message %q lacks %q", spec.ID, c.command, msg, expect)
			}
			if got[0].Level != ref[0].Level {
				t.Errorf("%s %q: level %v differs from the default harness %v", spec.ID, c.command, got[0].Level, ref[0].Level)
			}
			for _, mark := range aiTypography {
				if strings.Contains(msg, mark) {
					t.Errorf("%s %q: message carries %q: %q", spec.ID, c.command, mark, msg)
				}
			}
			if spec.ID == harness.ClaudeCode {
				continue
			}
			for _, token := range claudeOnly {
				if strings.Contains(msg, token) {
					t.Errorf("%s %q: message mentions %q: %q", spec.ID, c.command, token, msg)
				}
			}
		}
	}
}

func TestVocabularyIsComplete(t *testing.T) {
	for _, spec := range harness.All() {
		v := spec.Vocab
		for name, s := range map[string]string{
			"Background": v.Background, "Detach": v.Detach, "StopTracked": v.StopTracked,
			"ReadFile": v.ReadFile, "WriteTool": v.WriteTool, "EditTool": v.EditTool,
		} {
			if s == "" {
				t.Errorf("%s: vocabulary %s is empty", spec.ID, name)
			}
			for _, mark := range aiTypography {
				if strings.Contains(s, mark) {
					t.Errorf("%s: vocabulary %s carries %q", spec.ID, name, mark)
				}
			}
		}
		if spec.Label == "" || spec.SessionTag == nil || spec.TaskDir == nil {
			t.Errorf("%s: label, session tag or task dir missing", spec.ID)
		}
	}
}

func TestEveryMessageIsFreeOfAITypography(t *testing.T) {
	commands := []string{
		"server &", "nohup server", "pkill -f x", "cat notes.txt", "find . -name x", "grep foo a b",
		"sleep 90", "sleep 45", "ls /nix/store", "nixos-rebuild switch", "git add -A", "git reset --hard",
		"rg -rn foo .", "for x in a b; echo $x; end",
	}
	for _, spec := range harness.All() {
		cfg := &Settings{Spec: spec}
		for _, c := range commands {
			for _, v := range Evaluate(shell.Analyze(c), Context{}, cfg) {
				for _, mark := range aiTypography {
					if strings.Contains(v.Message, mark) {
						t.Errorf("%s %q %s: message carries %q", spec.ID, c, v.Check, mark)
					}
				}
			}
		}
	}
}

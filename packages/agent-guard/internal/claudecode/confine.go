package claudecode

import (
	"path/filepath"

	"agent-guard/internal/checks"
)

func ConfineRoots(agent, projectDir, home string) []string {
	return []string{
		filepath.Join(home, ".claude", "agent-memory", agent),
		filepath.Join(projectDir, ".claude", "agent-memory", agent),
		filepath.Join(projectDir, ".claude", "agent-memory-local", agent),
	}
}

func ProjectDir(getenv func(string) string, cwd string) string {
	if dir := getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		return dir
	}
	if root := checks.GitRoot(cwd); root != "" {
		return root
	}
	return cwd
}

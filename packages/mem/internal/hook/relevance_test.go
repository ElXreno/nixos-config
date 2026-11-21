package hook

import (
	"testing"
)

func TestSlashCommand(t *testing.T) {
	for _, c := range []struct {
		prompt string
		want   bool
	}{
		{"/code-review high", true},
		{"/clear", true},
		{"  /plugin-authoring:make x", true},
		{"/home/elxreno/x is broken", false},
		{"/nix/store/abc-foo/bin/foo fails", false},
		{"fix /etc/hosts", false},
		{"/ what", false},
	} {
		if got := isSlashCommand(c.prompt); got != c.want {
			t.Errorf("%q: %v, want %v", c.prompt, got, c.want)
		}
	}
}

func TestWithin(t *testing.T) {
	for _, c := range []struct {
		path, pattern string
		want          bool
	}{
		{"/h/.claude/projects/p/memory/a.md", "/h/.claude/projects/*/memory", true},
		{"/h/.claude/projects/p/memory", "/h/.claude/projects/*/memory", true},
		{"/h/.claude/projects/p/memory/sub/a.md", "/h/.claude/projects/*/memory", true},
		{"/h/.claude/projects/p/memory-old/a.md", "/h/.claude/projects/*/memory", false},
		{"/h/.claude/projects/p/q/memory/a.md", "/h/.claude/projects/*/memory", false},
		{"/h/.claude/projects/memory", "/h/.claude/projects/*/memory", false},
		{"/d/agent-memory/mem.db", "/d/agent-memory", true},
		{"/d/agent-memory2/mem.db", "/d/agent-memory", false},
		{"/d", "/d/agent-memory", false},
	} {
		if got := within(c.path, c.pattern); got != c.want {
			t.Errorf("within(%q, %q) = %v, want %v", c.path, c.pattern, got, c.want)
		}
	}
}

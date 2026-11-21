package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-guard/internal/harness"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func env(home string) func(string) string {
	return func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
}

func TestLoadResolvesHarnessAndHome(t *testing.T) {
	cfg, err := Load(write(t, `{"harness":"pi","scratchBase":"/s","reposRoot":"/r","avoidRoots":["/tmp"],"memoryPaths":[".claude/agent-memory/"]}`), env("/home/x"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.ID != harness.Pi || cfg.Home != "/home/x" || cfg.Checks().ScratchBase != "/s" || cfg.Checks().ReposRoot != "/r" || cfg.Checks().AvoidRoots[0] != "/tmp" {
		t.Errorf("got spec %q, home %q, settings %+v", cfg.Spec.ID, cfg.Home, cfg.Checks())
	}
	if cfg.EditOptions().Vocab.WriteTool != "write" || cfg.Memory() == nil {
		t.Errorf("edit options must carry the pi vocabulary and the memory paths, got %+v", cfg.EditOptions().Vocab)
	}
}

func TestLoadDefaultsToClaudeCodeWithoutAFile(t *testing.T) {
	cfg, err := Load("", env("/home/x"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.ID != harness.ClaudeCode || cfg.Home != "/home/x" || cfg.Memory() != nil {
		t.Errorf("got spec %q, home %q", cfg.Spec.ID, cfg.Home)
	}
}

func TestLoadKeepsAnExplicitHome(t *testing.T) {
	cfg, err := Load(write(t, `{"harness":"claude-code","home":"/home/test"}`), env("/home/x"))
	if err != nil || cfg.Home != "/home/test" {
		t.Errorf("got %+v, %v", cfg, err)
	}
}

func TestLoadIsStrict(t *testing.T) {
	for _, body := range []string{
		`{"harness":"pi-coding-agent"}`,
		`{"harness":"pi","scratchRoots":["/tmp"]}`,
		`{"home":"/home/test"}`,
	} {
		if _, err := Load(write(t, body), env("/home/x")); err == nil {
			t.Errorf("%s must be refused", body)
		}
	}
}

func TestCheck(t *testing.T) {
	good := `{"harness":"pi","journal":"/j.jsonl","scratchBase":"/s","shellReadRoots":["/etc"],"readOnlyRoots":[{"path":"/h/.claude","reason":"r"}],"deniedWrites":[{"path":"/proc/sys/vm/drop_caches","reason":"r"}],"protectedRepos":["github.com/o/*"]}`
	cfg, err := Load(write(t, good), env("/home/x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Check(); err != nil {
		t.Errorf("a valid config passes: %v", err)
	}
	bad := `{"harness":"pi","journal":"j.jsonl","scratchBase":"","createRoots":["~/x"],"readOnlyRoots":[{"path":"/h","reason":""}],"deniedWrites":[{"path":"vm/drop_caches","reason":""}],"grammars":{"go":"/nonexistent.so"},"protectedRepos":["github.com/[o"]}`
	cfg, err = Load(write(t, bad), env("/home/x"))
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Check()
	for _, want := range []string{`journal "j.jsonl"`, `scratchBase ""`, `createRoots entry "~/x"`, `"/h" has no reason`, `deniedWrites path "vm/drop_caches"`, `deniedWrites entry "vm/drop_caches" has no reason`, "grammar go", `protectedRepos entry "github.com/[o"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

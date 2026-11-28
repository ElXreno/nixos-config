package checks

import (
	"testing"

	"agent-guard/internal/shell"
)

func TestMemAgent(t *testing.T) {
	cfg := &Settings{Home: "/home/test", AgentStores: []string{"/home/test/.local/share/agent-memory", "/home/test/.local/state/agent-memory/agents"}}
	ctx := Context{Cwd: "/home/test/.local/share"}
	for _, tc := range []struct {
		want string
		cmd  string
	}{
		{"ask", `mem --as deepseek search zebra`},
		{"ask", `mem -as=deepseek get some_key`},
		{"ask", `mem --db /x/mem.db --as deepseek list`},
		{"ask", `MEM_AGENT=deepseek mem list`},
		{"ask", `env MEM_AGENT=deepseek mem search x`},
		{"ask", `export MEM_AGENT=deepseek; mem list`},
		{"ask", `bash -c "MEM_AGENT=deepseek mem get k"`},
		{"ask", `sqlite3 ~/.local/share/agent-memory/mem.db "select body from records"`},
		{"ask", `sqlite3 /home/test/.local/share/agent-memory/mem.db .dump | rg zebra`},
		{"ask", `strings agent-memory/mem.db-wal`},
		{"ask", `cat ~/.local/share/agent-memory/agents/deepseek/memory-core.md`},
		{"ask", `rg zebra /home/test/.local/state/agent-memory/agents`},
		{"ask", `cp --target-directory=/tmp/x ~/.local/share/agent-memory/mem.db`},
		{"ask", `wc -l < ~/.local/state/agent-memory/agents/deepseek/hook.log`},
		{"silent", `mem search zebra`},
		{"silent", `mem put k --kind fact --scope global --title T --body "about --as and MEM_AGENT"`},
		{"silent", `mem --db ~/.local/share/agent-memory/mem.db list`},
		{"silent", `mem get k -- --as`},
		{"silent", `echo "mem --as deepseek"`},
		{"silent", `cat ~/.local/state/agent-memory/hook.log`},
		{"silent", `ls ~/.local/share`},
		{"silent", `rg MEM_AGENT packages/mem`},
	} {
		got := runOneWith(MemAgent, shell.Analyze(tc.cmd), ctx, cfg).Worst().String()
		if got != tc.want {
			t.Errorf("want %s got %s for %q", tc.want, got, tc.cmd)
		}
	}
	if got := runOneWith(MemAgent, shell.Analyze(`mem --as deepseek list`), ctx, &Settings{Home: "/home/test"}).Worst().String(); got != "silent" {
		t.Errorf("the check fires without configured stores: %s", got)
	}
	if v := AgentStoreForFile("/home/test/.local/share/agent-memory/agents/deepseek/memory-core.md", ctx, cfg); v == nil || v.Check != MemAgent {
		t.Errorf("a file read of an agent core file = %+v", v)
	}
	if v := AgentStoreForFile("/home/test/projects/x/main.go", ctx, cfg); v != nil {
		t.Errorf("an ordinary file read = %+v", v)
	}
}

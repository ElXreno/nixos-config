package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgents(t *testing.T) {
	sb := newSandbox(t)
	primaryCore := sb.vars["MEM_CORE"]
	agentCore := filepath.Join(sb.dir, "data", "agent-memory", "agents", "deepseek", "memory-core.md")
	as := func(agent string) {
		if agent == "" {
			delete(sb.vars, "MEM_AGENT")
			sb.vars["MEM_CORE"] = primaryCore
			return
		}
		sb.vars["MEM_AGENT"] = agent
		delete(sb.vars, "MEM_CORE")
	}
	run := func(name string, code int, stdin string, has, lacks []string, args ...string) {
		t.Helper()
		got, out, errs := sb.run(stdin, args...)
		if got != code {
			t.Errorf("%s: exit %d, want %d (stdout %q, stderr %q)", name, got, code, out, errs)
		}
		for _, w := range has {
			if !strings.Contains(out+errs, w) {
				t.Errorf("%s: output lacks %q: %q %q", name, w, out, errs)
			}
		}
		for _, w := range lacks {
			if strings.Contains(out+errs, w) {
				t.Errorf("%s: output has %q: %q %q", name, w, out, errs)
			}
		}
	}
	file := func(path string) string {
		data, _ := os.ReadFile(path)
		return string(data)
	}

	run("primary put", 0, "Never push to the remote by default: wait until the owner asks for the push.", []string{"created"}, nil,
		"put", "push-when-asked", "--kind", "rule", "--scope", "global", "--title", "Push only when asked", "--core")
	as("deepseek")
	run("agent put", 0, "The zebra capture runs on port 7001 and is kept on this machine.", []string{"zebra-capture\tcreated"}, nil,
		"put", "zebra-capture", "--kind", "rule", "--scope", "global", "--title", "Zebra capture", "--core")
	run("agent reads the shared record", 0, "", []string{"Push only when asked"}, nil, "get", "push-when-asked")
	run("agent lists both", 0, "", []string{"zebra-capture", "push-when-asked"}, nil, "list")
	run("agent cannot change the shared record", 3, "changed", []string{"shared memory", "agent deepseek only reads"}, nil,
		"put", "push-when-asked", "--update")
	run("agent cannot borrow", 2, "", []string{"--as is for the primary agent"}, nil, "--as", "qwen", "list")
	if core := file(agentCore); !strings.Contains(core, "zebra-capture") || !strings.Contains(core, "push-when-asked") {
		t.Errorf("the agent core file = %q", core)
	}
	if core := file(primaryCore); strings.Contains(core, "zebra") || !strings.Contains(core, "push-when-asked") {
		t.Errorf("the primary core file after the agent wrote = %q", core)
	}

	as("")
	run("primary get", 1, "", []string{"not found"}, []string{"7001"}, "get", "zebra-capture")
	run("primary list", 0, "", []string{"push-when-asked"}, []string{"zebra"}, "list")
	run("primary search", 0, "", nil, []string{"zebra"}, "search", "--mode", "fused", "zebra capture port")
	run("primary history", 1, "", nil, []string{"7001"}, "history", "zebra-capture")
	run("primary topics", 0, "", nil, []string{"zebra"}, "topics", "list")
	run("primary key clash", 3, "A record of the primary agent.", []string{"taken by a record of another agent"}, []string{"deepseek"},
		"put", "zebra-capture", "--kind", "fact", "--scope", "global", "--title", "Mine")
	run("primary cannot tag", 2, "A record with an agent scope given by hand.", []string{"set by MEM_AGENT"}, nil,
		"put", "tagged", "--kind", "fact", "--scope", "global", "--scope", "agent:deepseek", "--title", "Tagged")
	run("borrowed get", 0, "", []string{"7001", "agent:deepseek"}, nil, "--as", "deepseek", "get", "zebra-capture")
	run("borrowed search", 0, "", []string{"zebra-capture"}, nil, "--as", "deepseek", "search", "--mode", "fused", "zebra capture port")
	run("borrowed write", 2, "changed", []string{"--as only reads"}, nil, "--as", "deepseek", "put", "zebra-capture", "--update")
	run("another agent's view", 1, "", []string{"not found"}, []string{"7001"}, "--as", "qwen", "get", "zebra-capture")
	if core := file(primaryCore); strings.Contains(core, "zebra") {
		t.Errorf("the primary core file after borrowed reads = %q", core)
	}
	run("share", 0, "", []string{"zebra-capture\tshared\tversion 2"}, nil, "--as", "deepseek", "share", "zebra-capture", "--why", "the owner asked")
	run("primary get after the share", 0, "", []string{"7001"}, []string{"agent:deepseek"}, "get", "zebra-capture")
	if core := file(primaryCore); !strings.Contains(core, "zebra-capture") {
		t.Errorf("the primary core file after the share = %q", core)
	}
}

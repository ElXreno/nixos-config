package checks

import (
	"path/filepath"
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

const MemAgent = "mem-agent"

const memAgentMsg = "This reaches into the memory of another agent: "

const memAgentTail = ". Those records come from the owner's local-model sessions and stay out of this session unless the owner allows this call."

var envSetters = map[string]bool{"export": true, "declare": true, "typeset": true, "local": true, "readonly": true, "set": true}

// memAgent asks before a command widens what mem shows (--as, MEM_AGENT) or
// reads the store files of other agents with anything but mem itself.
func memAgent(a *shell.Analysis, ctx Context, cfg *Settings) *verdict.Verdict {
	if len(cfg.AgentStores) == 0 {
		return nil
	}
	for _, cmd := range a.Commands {
		if why := memAgentReason(cmd, ctx, cfg); why != "" {
			return ask(memAgentMsg + why + memAgentTail)
		}
	}
	return nil
}

func memAgentReason(cmd shell.Command, ctx Context, cfg *Settings) string {
	if _, ok := cmd.Env["MEM_AGENT"]; ok {
		return "the command sets MEM_AGENT"
	}
	head, args := cmd.Head(), cmd.Args()
	if lead := len(cmd.Argv) - 1 - len(args); lead > 0 {
		for _, a := range cmd.Argv[:lead] {
			if strings.HasPrefix(a, "MEM_AGENT=") {
				return "the command sets MEM_AGENT"
			}
		}
	}
	if envSetters[head] || head == "env" {
		for _, a := range args {
			if a == "MEM_AGENT" || strings.HasPrefix(a, "MEM_AGENT=") {
				return "the command sets MEM_AGENT"
			}
		}
	}
	if head == "mem" {
		for _, a := range args {
			if a == "--" {
				break
			}
			if name := strings.TrimLeft(a, "-"); name != a && (name == "as" || strings.HasPrefix(name, "as=")) {
				return "mem --as"
			}
		}
		return ""
	}
	words := append([]string{}, args...)
	words = append(words, cmd.Stdin, cmd.StdoutTo)
	for _, r := range cmd.Redirects {
		words = append(words, r.Target)
	}
	for _, w := range words {
		if p := agentStorePath(w, ctx.Cwd, cfg); p != "" {
			return head + " reads " + p
		}
	}
	return ""
}

func agentStorePath(word, cwd string, cfg *Settings) string {
	if i := strings.IndexByte(word, '='); i > 0 && strings.HasPrefix(word, "-") {
		word = word[i+1:]
	}
	if word == "" {
		return ""
	}
	p := cfg.expand(word)
	if !filepath.IsAbs(p) {
		if cwd == "" {
			return ""
		}
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if cfg.underAny(p, cfg.AgentStores) {
		return p
	}
	return ""
}

// AgentStoreForFile is memAgent for a file tool that reads a path.
func AgentStoreForFile(path string, ctx Context, cfg *Settings) *verdict.Verdict {
	if len(cfg.AgentStores) == 0 || path == "" {
		return nil
	}
	if p := agentStorePath(path, ctx.Cwd, cfg); p != "" {
		v := ask(memAgentMsg + "the tool reads " + p + memAgentTail)
		v.Check = MemAgent
		return v
	}
	return nil
}

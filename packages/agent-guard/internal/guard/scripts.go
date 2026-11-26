package guard

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"agent-guard/internal/call"
	"agent-guard/internal/checks"
	"agent-guard/internal/edit"
	"agent-guard/internal/shell"
	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

func (e *Engine) scriptPath(p, cwd string) (string, bool) {
	if p == "" || strings.ContainsAny(p, "$`*?") {
		return "", false
	}
	if strings.HasPrefix(p, "~/") && e.Checks != nil && e.Checks.Home != "" {
		p = filepath.Join(e.Checks.Home, p[2:])
	}
	if !filepath.IsAbs(p) {
		if cwd == "" {
			return "", false
		}
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	if strings.HasPrefix(p, "/nix/store/") || strings.HasPrefix(p, "/run/") {
		return "", false
	}
	return p, true
}

func (e *Engine) scripts(a *shell.Analysis, ctx checks.Context, seen map[string]bool, depth int) verdict.Verdicts {
	if e.ReadScript == nil || depth >= shell.MaxDepth {
		return nil
	}
	var out verdict.Verdicts
	checks.WalkDirs(a, ctx.Cwd, e.Checks, func(cmd shell.Command, cwd string, known bool) {
		if cmd.Kind != shell.Local {
			return
		}
		raw, direct, ok := shell.ScriptFile(cmd)
		if !ok {
			return
		}
		if !known {
			cwd = ""
		}
		p, ok := e.scriptPath(raw, cwd)
		if !ok || seen[p] {
			return
		}
		seen[p] = true
		src, ok := e.ReadScript(p)
		if !ok || strings.ContainsRune(src, 0) || (direct && !shell.ShellShebang(src)) {
			return
		}
		sub, here := shell.Analyze(src), ctx
		here.Cwd = cwd
		found := append(checks.Evaluate(sub, here, e.Checks), e.scripts(sub, here, seen, depth+1)...)
		for _, v := range found {
			if !scriptCheck(v.Check) {
				continue
			}
			v.Message = "In script " + p + ": " + v.Message
			v.Fix = ""
			out = append(out, v)
		}
	})
	return out
}

func scriptCheck(name string) bool {
	switch name {
	case checks.Parser, checks.Background, checks.Sleep, checks.ShellTool:
		return false
	}
	return true
}

func shellSource(path, src string) bool {
	switch filepath.Ext(path) {
	case ".sh", ".bash":
		return true
	}
	return shell.ShellShebang(src)
}

func (e *Engine) snippets(path, src string) []string {
	if src == "" {
		return nil
	}
	if shellSource(path, src) {
		return []string{src}
	}
	found, err := e.Edit.Grammars.ShellSnippets(path, []byte(src))
	if err != nil {
		return nil
	}
	out := make([]string, len(found))
	for i, s := range found {
		out[i] = s.Text
	}
	return out
}

func scriptFindings(snippets []string, cfg *checks.Settings) map[string]verdict.Verdict {
	out := map[string]verdict.Verdict{}
	for _, s := range snippets {
		for _, cmd := range shell.Analyze(s).Commands {
			for _, v := range checks.Script(cmd, cfg) {
				out[v.Check+"\x00"+cmd.Raw] = v
			}
		}
	}
	return out
}

func (e *Engine) scriptAdvice(c call.Change, opts edit.Options) verdict.Verdicts {
	after, ok := edit.After(c, opts)
	if !ok {
		return nil
	}
	found := scriptFindings(e.snippets(c.Path, after), e.Checks)
	if len(found) == 0 {
		return nil
	}
	before := scriptFindings(e.snippets(c.Path, edit.Before(c, opts)), e.Checks)
	keys := slices.Sorted(maps.Keys(found))
	var out verdict.Verdicts
	for _, key := range keys {
		v := found[key]
		if _, old := before[key]; old {
			continue
		}
		cmdText := key[strings.IndexByte(key, 0)+1:]
		out = append(out, verdict.Verdict{
			Level:   verdict.Advise,
			Check:   v.Check,
			Message: "This script in " + c.Path + " will run `" + text.Clip(cmdText, 160) + "`, which the shell guard flags: " + v.Message,
		})
	}
	return out
}

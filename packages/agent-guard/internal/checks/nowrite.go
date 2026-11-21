package checks

import (
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

type DeniedWrite struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func (cfg *Settings) deniedWrite(p string) (DeniedWrite, bool) {
	for _, d := range cfg.DeniedWrites {
		if cfg.underAny(p, []string{d.Path}) {
			return d, true
		}
	}
	return DeniedWrite{}, false
}

func sysctlTargets(args []string) []string {
	var out []string
	for _, p := range positionals(args) {
		key, _, ok := strings.Cut(p, "=")
		if !ok || key == "" || strings.HasPrefix(key, "/") {
			continue
		}
		out = append(out, "/proc/sys/"+strings.ReplaceAll(key, ".", "/"))
	}
	return out
}

func noWrite(a *shell.Analysis, ctx Context, cfg *Settings) *verdict.Verdict {
	if len(cfg.DeniedWrites) == 0 {
		return nil
	}
	var paths []string
	var rule DeniedWrite
	seen := map[string]bool{}
	remoteCwd := ""
	walkDirs(a, ctx.Cwd, cfg, func(cmd shell.Command, cwd string, _ bool) {
		if cmd.Kind == shell.Remote {
			if cmd.Head() == "cd" {
				remoteCwd = ""
				if pos := positionals(cmd.Args()); len(pos) > 0 {
					remoteCwd = cfg.resolve(pos[0], remoteCwd)
				}
				return
			}
			cwd = remoteCwd
		}
		var targets []string
		for _, t := range createTargets(cmd, cwd, cfg) {
			targets = append(targets, t.path)
		}
		if cmd.Head() == "sysctl" {
			targets = append(targets, sysctlTargets(cmd.Args())...)
		}
		for _, t := range targets {
			p := cfg.resolve(t, cwd)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			if d, ok := cfg.deniedWrite(p); ok {
				if len(paths) == 0 {
					rule = d
				}
				paths = append(paths, p)
			}
		}
	})
	if len(paths) == 0 {
		return nil
	}
	return deny("Blocked: writing " + strings.Join(paths, ", ") + ", on this host or any other: " + rule.Reason)
}

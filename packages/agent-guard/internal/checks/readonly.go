package checks

import (
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

type ReadOnlyRoot struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func (cfg *Settings) readOnlyRoot(p string) (ReadOnlyRoot, bool) {
	for _, r := range cfg.ReadOnlyRoots {
		if cfg.underAny(p, []string{r.Path}) {
			return r, true
		}
	}
	return ReadOnlyRoot{}, false
}

func readOnlyMsg(paths []string, r ReadOnlyRoot, cfg *Settings) string {
	return "Blocked: " + strings.Join(paths, ", ") + " is under " + cfg.expand(r.Path) + ", which is read-only here: " + r.Reason
}

func readOnly(a *shell.Analysis, ctx Context, cfg *Settings) *verdict.Verdict {
	if len(cfg.ReadOnlyRoots) == 0 {
		return nil
	}
	var paths []string
	var root ReadOnlyRoot
	walkTargets(a, ctx.Cwd, cfg, func(p, _ string, _ target) {
		if r, ok := cfg.readOnlyRoot(p); ok {
			if len(paths) == 0 {
				root = r
			}
			paths = append(paths, p)
		}
	})
	if len(paths) == 0 {
		return nil
	}
	return deny(readOnlyMsg(paths, root, cfg))
}

func ReadOnlyForFile(path string, ctx Context, cfg *Settings) *verdict.Verdict {
	p := cfg.resolve(path, ctx.Cwd)
	if p == "" {
		return nil
	}
	r, ok := cfg.readOnlyRoot(p)
	if !ok {
		return nil
	}
	v := deny(readOnlyMsg([]string{p}, r, cfg))
	v.Check = ReadOnly
	return v
}

package checks

import (
	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

func runOne(name string, a *shell.Analysis, cfg *Settings) verdict.Verdicts {
	return runOneWith(name, a, Context{}, cfg)
}

func runOneWith(name string, a *shell.Analysis, ctx Context, cfg *Settings) verdict.Verdicts {
	var out verdict.Verdicts
	for _, v := range Evaluate(a, ctx, cfg) {
		if v.Check == name {
			out = append(out, v)
		}
	}
	return out
}

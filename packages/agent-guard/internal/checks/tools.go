package checks

import (
	"regexp"

	"agent-guard/internal/harness"
	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

var grepLongArgs = regexp.MustCompile(`^--(regexp|file|after-context|before-context|context|max-count|exclude|include|exclude-dir|exclude-from|devices|directories|binary-files|label|group-separator)$`)

const findMsg = "Blocked: use fd instead of find - it is the standard here and far faster."

func catMsg(v harness.Vocabulary) string {
	return "cat ran, but a file you may edit next " + v.ReadFile + " cat is fine for one-off looks inside a command chain."
}

const grepMsg = "Blocked: use rg instead of grep for searching files - it is the standard here and far faster. Remember rg -r is --replace, not recursive. grep stays fine for filtering a stream in a pipeline."

func shellTool(cmd shell.Command, cfg *Settings) *verdict.Verdict {
	if cmd.Kind == shell.Remote {
		return nil
	}
	switch cmd.Head() {
	case "find":
		return deny(findMsg)
	case "cat":
		if cmd.InPipeline || len(cmd.Redirects) > 0 {
			return nil
		}
		files := positionals(cmd.Args())
		if len(files) == 0 || cfg.allUnder(files, cfg.ShellReadRoots) {
			return nil
		}
		return advise(catMsg(cfg.spec().Vocab))
	case "grep":
		if cmd.InPipeline || len(cmd.Redirects) > 0 {
			return nil
		}
		files := optPositionals(cmd.Args(), "efABCmdD", grepLongArgs)
		if len(files) < 2 || cfg.allUnder(files[1:], cfg.ShellReadRoots) {
			return nil
		}
		return deny(grepMsg)
	}
	return nil
}

func (cfg *Settings) allUnder(paths []string, roots []string) bool {
	for _, p := range paths {
		if !cfg.underAny(p, roots) {
			return false
		}
	}
	return true
}

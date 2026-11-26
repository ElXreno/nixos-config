// Package checks holds the shell guards. A check looks at one simple command or
// at the whole analysis and returns at most one verdict; Evaluate aggregates
// them.
package checks

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"agent-guard/internal/harness"
	"agent-guard/internal/shell"
	"agent-guard/internal/text"
	"agent-guard/internal/verdict"
)

type Settings struct {
	Home           string
	ShellReadRoots []string
	CreateRoots    []string
	ReadOnlyRoots  []ReadOnlyRoot
	DeniedWrites   []DeniedWrite
	ScratchBase    string
	ReposRoot      string
	AvoidRoots     []string
	ProtectedRepos []string
	Spec           harness.Spec
}

func (cfg *Settings) spec() harness.Spec {
	if cfg.Spec.ID != "" {
		return cfg.Spec
	}
	s, _ := harness.ByID("")
	return s
}

const (
	RgReplace      = "rg-replace"
	GitScope       = "git-scope"
	GitDestructive = "git-destructive"
	GitPush        = "git-push"
	NixStoreRoot   = "nix-store-root"
	NixOutputs     = "nix-outputs"
	ShellTool      = "shell-tool"
	Sleep          = "sleep"
	Background     = "background"
	Pkill          = "pkill"
	Deploy         = "deploy"
	Parser         = "parser"
	ReadOnly       = "read-only-root"
	NoWrite        = "no-write"
	Location       = "location"
)

type check struct {
	name     string
	command  func(cmd shell.Command, cfg *Settings) *verdict.Verdict
	analysis func(a *shell.Analysis, ctx Context, cfg *Settings) *verdict.Verdict
}

var registry = []check{
	{name: RgReplace, command: rgReplace},
	{name: GitScope, command: gitScope},
	{name: GitDestructive, command: gitDestructive},
	{name: NixStoreRoot, command: nixStoreRoot},
	{name: NixOutputs, command: nixOutputs},
	{name: ShellTool, command: shellTool},
	{name: Sleep, command: sleepCheck},
	{name: Background, command: background},
	{name: Pkill, command: pkill},
	{name: Deploy, command: deploy},
	{name: Parser, analysis: parser},
	{name: ReadOnly, analysis: readOnly},
	{name: NoWrite, analysis: noWrite},
	{name: Location, analysis: location},
	{name: GitPush, analysis: gitPush},
}

func Names() []string {
	out := make([]string, len(registry))
	for i, c := range registry {
		out[i] = c.name
	}
	return out
}

// Evaluate runs every per-command check against every command of the
// analysis, nested payloads included, then the checks that need the whole
// analysis.
func Evaluate(a *shell.Analysis, ctx Context, cfg *Settings) verdict.Verdicts {
	var out verdict.Verdicts
	for _, cmd := range a.Commands {
		for _, c := range registry {
			if c.command == nil {
				continue
			}
			if v := c.command(cmd, cfg); v != nil {
				v.Check = c.name
				out = append(out, *v)
			}
		}
	}
	for _, c := range registry {
		if c.analysis == nil {
			continue
		}
		if v := c.analysis(a, ctx, cfg); v != nil {
			v.Check = c.name
			out = append(out, *v)
		}
	}
	if fix, ok := rgFix(a); ok {
		for i := range out {
			if out[i].Check == RgReplace {
				out[i].Fix = fix
				break
			}
		}
	}
	return out
}

func parser(a *shell.Analysis, _ Context, _ *Settings) *verdict.Verdict {
	var depth *shell.Unparsed
	for i, u := range a.Unparsed {
		if u.Reason == shell.ReasonParse {
			reason := ""
			if u.Err != nil {
				reason = " (" + text.Clip(strings.SplitN(u.Err.Error(), "\n", 2)[0], 160) + ")"
			}
			return advise("This command did not parse as bash" + reason + ", so the guards only ran their conservative fallback on it. If it is fish or another dialect, wrap that part in bash -c \"...\"; if it is a typo, fix it.")
		}
		if depth == nil {
			depth = &a.Unparsed[i]
		}
	}
	if depth == nil {
		return nil
	}
	return advise("This command nests interpreters more than " + strconv.Itoa(shell.MaxDepth) + " levels deep, so the guards did not look inside the innermost payload: " + text.Clip(depth.Text, 120) + ". Run that part as a command of its own.")
}

func deny(msg string) *verdict.Verdict { return &verdict.Verdict{Level: verdict.Deny, Message: msg} }
func ask(msg string) *verdict.Verdict  { return &verdict.Verdict{Level: verdict.Ask, Message: msg} }
func advise(msg string) *verdict.Verdict {
	return &verdict.Verdict{Level: verdict.Advise, Message: msg}
}

// positionals returns the arguments that are not options, honouring "--".
func positionals(args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--" {
			return append(out, args[i+1:]...)
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// optPositionals is positionals for programs whose options take arguments: a
// two-character short option listed in shortArgs, or a long option matched by
// longArgs without an "=", consumes the following word.
func optPositionals(args []string, shortArgs string, longArgs *regexp.Regexp) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(out, args[i+1:]...)
		}
		if strings.HasPrefix(a, "--") {
			if !strings.Contains(a, "=") && longArgs != nil && longArgs.MatchString(a) {
				i++
			}
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			if len(a) == 2 && strings.ContainsRune(shortArgs, rune(a[1])) {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func shortCluster(a string) (string, bool) {
	if len(a) < 2 || a[0] != '-' || a[1] == '-' {
		return "", false
	}
	return a[1:], true
}

func hasShortFlag(args []string, letter byte) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if cluster, ok := shortCluster(a); ok {
			for i := 0; i < len(cluster); i++ {
				if cluster[i] == letter {
					return true
				}
				if cluster[i] < 'A' || (cluster[i] > 'Z' && cluster[i] < 'a') || cluster[i] > 'z' {
					break
				}
			}
		}
	}
	return false
}

func contains(args []string, want ...string) bool {
	for _, a := range args {
		for _, w := range want {
			if a == w {
				return true
			}
		}
	}
	return false
}

func (cfg *Settings) expand(p string) string {
	switch {
	case cfg.Home == "":
	case p == "~":
		return cfg.Home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(cfg.Home, p[2:])
	}
	return p
}

func (cfg *Settings) underAny(p string, roots []string) bool {
	p = cfg.expand(p)
	for _, r := range roots {
		r = cfg.expand(r)
		if p == r || strings.HasPrefix(p, strings.TrimSuffix(r, "/")+"/") {
			return true
		}
	}
	return false
}

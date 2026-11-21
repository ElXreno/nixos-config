package checks

import (
	"regexp"
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

const rgReplaceMsg = "Blocked: rg -r is --replace, NOT recursive (recursion is default in rg). Drop the -r flag. If you really meant text replacement, use --replace=<text> explicitly."

func rgClusters(args []string) []string {
	var out []string
	for _, a := range args {
		if a == "--" {
			break
		}
		if c, ok := shortCluster(a); ok && strings.ContainsRune(c, 'r') && lettersOnly(c) {
			out = append(out, a)
		}
	}
	return out
}

func rgReplace(cmd shell.Command, _ *Settings) *verdict.Verdict {
	if cmd.Head() != "rg" || len(rgClusters(cmd.Args())) == 0 {
		return nil
	}
	return deny(rgReplaceMsg)
}

// rgFix turns the grep habit rg -rn into rg -n: in rg, -r is --replace and
// recursion is the default, so dropping the r is exactly what was meant. Only
// a top-level rg with one such flag cluster, written once as a bare word in the
// command, is fixed; anything else stays a deny.
func rgFix(a *shell.Analysis) (string, bool) {
	cluster := ""
	for _, cmd := range a.Commands {
		if cmd.Head() != "rg" {
			continue
		}
		for _, arg := range rgClusters(cmd.Args()) {
			c := arg[1:]
			if cluster != "" || cmd.Depth > 0 || cmd.Kind != shell.Local || len(c) < 2 || strings.Count(c, "r") > 1 {
				return "", false
			}
			cluster = arg
		}
	}
	if cluster == "" {
		return "", false
	}
	word := regexp.MustCompile(`(^|\s)` + regexp.QuoteMeta(cluster) + `(\s|$)`)
	if len(word.FindAllStringIndex(a.Source, -1)) != 1 {
		return "", false
	}
	start := word.FindStringSubmatchIndex(a.Source)[3]
	fixed := "-" + strings.Replace(cluster[1:], "r", "", 1)
	return a.Source[:start] + fixed + a.Source[start+len(cluster):], true
}

func lettersOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

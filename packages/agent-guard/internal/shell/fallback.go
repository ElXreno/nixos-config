package shell

import (
	"regexp"
	"strings"
)

var (
	quotedRe   = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'[^']*'`)
	redirAmpRe = regexp.MustCompile(`[0-9]*[<>]&[0-9-]+|&>>?|\|&|&&`)
	wordRe     = regexp.MustCompile(`(?:^|[\s;|&(])(nohup|setsid|disown|pkill|killall)(?:\s|$)`)
	sleepRe    = regexp.MustCompile(`(?:^|[\s;|&(])sleep\s+([0-9]+(?:\.[0-9]+)?[smhd]?)`)
)

// fallbackCommands is the conservative reading of text the parser rejected: it
// only reports the deny-class facts a regex can still see.
func fallbackCommands(src string, kind Kind, depth int) []Command {
	scan := quotedRe.ReplaceAllString(src, "Q")
	scan = redirAmpRe.ReplaceAllString(scan, " ")
	var out []Command
	if strings.Contains(scan, "&") {
		out = append(out, Command{Kind: kind, Origin: Fallback, Background: true, Raw: src, Depth: depth})
	}
	for _, m := range wordRe.FindAllStringSubmatch(scan, -1) {
		switch m[1] {
		case "nohup", "setsid", "disown":
			out = append(out, Command{Kind: kind, Origin: Fallback, Argv: []string{m[1]}, Detached: true, Raw: src, Depth: depth})
		default:
			out = append(out, Command{Kind: kind, Origin: Fallback, Argv: []string{m[1]}, Raw: src, Depth: depth})
		}
	}
	for _, m := range sleepRe.FindAllStringSubmatch(scan, -1) {
		out = append(out, Command{Kind: kind, Origin: Fallback, Argv: []string{"sleep", m[1]}, Raw: src, Depth: depth})
	}
	for i := range out {
		out[i].settle()
	}
	return out
}

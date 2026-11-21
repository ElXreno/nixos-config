package checks

import (
	"regexp"
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

var storeGlob = regexp.MustCompile(`[*?\[{]`)

var storeWalkers = map[string]bool{
	"ls": true, "find": true, "fd": true, "fdfind": true, "du": true, "dua": true, "ncdu": true, "tree": true,
	"rsync": true, "cp": true, "tar": true, "dust": true, "eza": true, "exa": true, "diskus": true,
}

var storeSearchers = map[string]bool{"rg": true, "grep": true, "ag": true, "ack": true, "egrep": true, "fgrep": true}

const nixStoreMsg = `Blocked: no traversing the root of /nix or /nix/store: the store holds hundreds of thousands of entries and a sweep costs minutes of disk IO. A concrete entry is fine, globs inside it included: /nix/store/<hash>-<name>/**/*.so
Derive the path instead of searching for it:
  nix build <attr> --no-link --print-out-paths
  nix eval <attr> --raw
  nix path-info <attr>            (-r for the closure, -S for sizes)
  nix derivation show <attr>
  nix flake metadata <flake> --json | jq -r .path
  nix-store -q --references / --requisites <path>
  nix why-depends <from> <to>
  readlink -f on result, /run/current-system, ~/.nix-profile`

func nixStoreRoot(cmd shell.Command, _ *Settings) *verdict.Verdict {
	head := cmd.Head()
	skipPattern := false
	switch {
	case storeWalkers[head]:
	case storeSearchers[head]:
		skipPattern = true
	default:
		return nil
	}
	pos := positionals(cmd.Args())
	if skipPattern && len(pos) > 0 {
		pos = pos[1:]
	}
	for _, t := range pos {
		t = strings.Trim(t, `"'`)
		switch {
		case t == "/nix" || t == "/nix/":
			return deny(nixStoreMsg)
		case strings.HasPrefix(t, "/nix/store"):
			rest := strings.TrimPrefix(strings.TrimPrefix(t, "/nix/store"), "/")
			first := rest
			if i := strings.IndexByte(rest, '/'); i >= 0 {
				first = rest[:i]
			}
			switch {
			case first == "" || first == "." || first == ".." || first == ".links" || first == "trash" || storeGlob.MatchString(first):
				return deny(nixStoreMsg)
			}
		}
	}
	return nil
}

package checks

import (
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

const nixOutputsMsg = "Blocked: `nix build --print-out-paths` prints one path per output of each installable, and out does not come first (a -man, -dev or -lib path can), so `| head -1` or `$(...)` here takes whichever output happens to lead. Name the output on every installable: `nixpkgs#foo^out`, `^bin`, `^lib`, or the attribute form `.#pkg.bin`; `nix eval <installable>.outputs --json` lists them. Unnamed: "

var nixOneArg = map[string]bool{
	"-o": true, "--out-link": true, "-f": true, "--file": true, "--expr": true, "-I": true, "--include": true,
	"-j": true, "--max-jobs": true, "--cores": true, "--store": true, "--eval-store": true, "--builders": true,
	"--profile": true, "--log-format": true, "--reference-lock-file": true, "--output-lock-file": true,
	"--inputs-from": true, "--update-input": true, "--commit-lock-file-summary": true, "--keep-failed-dir": true,
}

var nixTwoArgs = map[string]bool{
	"--option": true, "--arg": true, "--argstr": true, "--arg-from-file": true, "--arg-from-stdin": true,
	"--override-input": true, "--override-flake": true,
}

var nixOutputNames = map[string]bool{
	"out": true, "bin": true, "lib": true, "dev": true, "man": true, "doc": true, "info": true, "devdoc": true,
	"static": true, "debug": true, "lib32": true, "devman": true, "python": true, "modules": true, "data": true,
	"terminfo": true, "examples": true, "first": true, "jar": true, "dist": true,
}

var lineTakers = map[string]bool{
	shell.Substitution: true, "?": true, "while": true, "head": true, "tail": true, "sed": true, "awk": true,
	"gawk": true, "cut": true, "xargs": true, "read": true, "mapfile": true, "readarray": true, "parallel": true,
}

func nixOutputs(cmd shell.Command, _ *Settings) *verdict.Verdict {
	if cmd.Head() != "nix" || !lineTakers[cmd.StdoutTo] {
		return nil
	}
	args := cmd.Args()
	if len(args) == 0 || args[0] != "build" || !contains(args, "--print-out-paths") {
		return nil
	}
	installables := nixInstallables(args[1:])
	if len(installables) == 0 {
		installables = []string{"."}
	}
	var unnamed []string
	for _, i := range installables {
		if !namesOutput(i) {
			unnamed = append(unnamed, i)
		}
	}
	if len(unnamed) == 0 {
		return nil
	}
	return deny(nixOutputsMsg + strings.Join(unnamed, ", ") + ".")
}

func nixInstallables(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return append(out, args[i+1:]...)
		case nixTwoArgs[a]:
			i += 2
		case nixOneArg[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			out = append(out, a)
		}
	}
	return out
}

func namesOutput(installable string) bool {
	i := strings.Trim(installable, `"'`)
	if strings.Contains(i, "^") || (strings.HasPrefix(i, "/nix/store/") && !strings.HasSuffix(i, ".drv")) {
		return true
	}
	attr := i
	if h := strings.LastIndexByte(i, '#'); h >= 0 {
		attr = i[h+1:]
	}
	if d := strings.LastIndexByte(attr, '.'); d >= 0 {
		return nixOutputNames[attr[d+1:]]
	}
	return false
}

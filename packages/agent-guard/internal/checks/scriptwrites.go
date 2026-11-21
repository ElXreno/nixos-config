package checks

import "regexp"

var (
	pyOpenLiteral = regexp.MustCompile(`\bopen\(\s*[rbfu]?\\?["']([^"'\\]+)\\?["']\s*,\s*[rbfu]?\\?["'](?:[wax]|r\+)`)
	pyPathLiteral = regexp.MustCompile(`Path\(\s*[rbfu]?\\?["']([^"'\\]+)\\?["']\s*\)\s*\.(?:write_(?:text|bytes)|open)\(`)

	// Node: the fs writers, sync and promise flavours alike.
	nodeLiteral = regexp.MustCompile(`\b(?:writeFileSync|writeFile|appendFileSync|appendFile|createWriteStream|copyFileSync|copyFile)\(\s*\\?["'` + "`" + `]([^"'` + "`" + `\\]+)`)
)

// dataSinks receive heredocs as bytes, not as code to run.
var dataSinks = map[string]bool{
	"cat": true, "tee": true, "dd": true, "cp": true, "install": true, "sponge": true, "base64": true,
	"gzip": true, "zstd": true, "xz": true, "sha256sum": true, "md5sum": true, "wc": true, "head": true,
	"tail": true, "sort": true, "uniq": true, "grep": true, "rg": true, "sed": true, "awk": true, "jq": true,
	"yq": true, "diff": true, "patch": true, "git": true, "curl": true, "wget": true, "mail": true, "xclip": true,
	"wl-copy": true, "nc": true, "socat": true, "psql": true, "sqlite3": true, "mysql": true, "openssl": true,
	"gpg": true, "age": true, "sops": true, "nixfmt": true, "tr": true, "cut": true,
	"column": true, "fold": true, "od": true, "xxd": true, "hexdump": true, "less": true, "more": true,
}

// scriptWriteTargets lists the file names a script visibly writes.
func scriptWriteTargets(code string) []string {
	var targets []string
	for _, re := range []*regexp.Regexp{pyOpenLiteral, pyPathLiteral, nodeLiteral} {
		for _, m := range re.FindAllStringSubmatch(code, -1) {
			targets = append(targets, m[1])
		}
	}
	return targets
}

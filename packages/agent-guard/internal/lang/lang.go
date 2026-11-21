package lang

import (
	"path/filepath"
	"strings"
)

type Class int

const (
	None Class = iota
	Config
	Hash
	Slash
	Dash
)

type Entry struct {
	Grammar string
	Class   Class
}

var exts = map[string]Entry{
	".bash": {"bash", Hash}, ".sh": {"bash", Hash}, ".zsh": {"bash", Hash}, ".env": {"bash", Config},
	".c": {"c", Slash}, ".h": {"c", Slash},
	".cs":    {"c-sharp", Slash},
	".cmake": {"cmake", Hash},
	".cc":    {"cpp", Slash}, ".cpp": {"cpp", Slash}, ".hpp": {"cpp", Slash},
	".dart": {"dart", Slash},
	".elm":  {"elm", Dash},
	".ex":   {"elixir", Hash}, ".exs": {"elixir", Hash},
	".fish":      {"fish", Hash},
	".gitignore": {"gitignore", Config},
	".go":        {"go", Slash},
	".hs":        {"haskell", Dash},
	".hcl":       {"hcl", Config}, ".tf": {"hcl", Config}, ".tfvars": {"hcl", Config},
	".cfg": {"ini", Config}, ".conf": {"ini", Config}, ".editorconfig": {"ini", Config}, ".ini": {"ini", Config},
	".java": {"java", Slash},
	".cjs":  {"javascript", Slash}, ".js": {"javascript", Slash}, ".jsx": {"javascript", Slash}, ".mjs": {"javascript", Slash},
	".jl":         {"julia", Hash},
	".kt":         {"kotlin", Slash},
	".lua":        {"lua", Dash},
	".mk":         {"make", Hash},
	".nix":        {"nix", Hash},
	".pl":         {"perl", Hash},
	".php":        {"php", Slash},
	".pp":         {"", Hash},
	".properties": {"properties", Config},
	".proto":      {"proto", Slash},
	".py":         {"python", Hash},
	".qml":        {"qmljs", Slash},
	".r":          {"r", Hash},
	".rb":         {"ruby", Hash},
	".rs":         {"rust", Slash},
	".scala":      {"scala", Slash},
	".sql":        {"sql", Dash},
	".swift":      {"swift", Slash},
	".toml":       {"toml", Config},
	".tsx":        {"tsx", Slash},
	".ts":         {"typescript", Slash},
	".yaml":       {"yaml", Config}, ".yml": {"yaml", Config},
	".zig": {"zig", Slash},
}

func dockerfile(base string) bool {
	return base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.")
}

func Of(path string) Entry {
	base := filepath.Base(path)
	e := exts[strings.ToLower(filepath.Ext(path))]
	switch {
	case base == "Makefile" || base == "GNUmakefile":
		e.Grammar = "make"
	case dockerfile(base):
		e.Grammar = "dockerfile"
	}
	switch {
	case e.Class == Config || strings.HasPrefix(base, ".env"):
		e.Class = Config
	case e.Class == Hash || dockerfile(base) || base == "Makefile":
		e.Class = Hash
	}
	return e
}

func Grammars() []string {
	seen := map[string]bool{"make": true, "dockerfile": true}
	out := []string{"dockerfile", "make"}
	for _, e := range exts {
		if e.Grammar != "" && !seen[e.Grammar] {
			seen[e.Grammar] = true
			out = append(out, e.Grammar)
		}
	}
	return out
}

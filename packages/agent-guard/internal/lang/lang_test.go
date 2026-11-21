package lang

import (
	"path/filepath"
	"strings"
	"testing"
)

var oldByExt = map[string]string{
	".sh": "bash", ".bash": "bash", ".zsh": "bash", ".env": "bash", ".c": "c", ".h": "c", ".cs": "c-sharp",
	".cmake": "cmake", ".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp", ".dart": "dart", ".ex": "elixir", ".exs": "elixir",
	".elm": "elm", ".fish": "fish", ".gitignore": "gitignore", ".go": "go", ".hs": "haskell", ".tf": "hcl",
	".tfvars": "hcl", ".hcl": "hcl", ".ini": "ini", ".cfg": "ini", ".conf": "ini", ".editorconfig": "ini",
	".java": "java", ".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".jl": "julia", ".kt": "kotlin", ".lua": "lua", ".mk": "make", ".nix": "nix", ".pl": "perl", ".php": "php",
	".properties": "properties", ".proto": "proto", ".py": "python", ".qml": "qmljs", ".r": "r", ".rb": "ruby",
	".rs": "rust", ".scala": "scala", ".sql": "sql", ".swift": "swift", ".toml": "toml", ".tsx": "tsx",
	".ts": "typescript", ".yaml": "yaml", ".yml": "yaml", ".zig": "zig",
}

func oldGrammarFor(path string) string {
	base := filepath.Base(path)
	switch {
	case base == "Makefile" || base == "GNUmakefile":
		return "make"
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		return "dockerfile"
	}
	return oldByExt[strings.ToLower(filepath.Ext(path))]
}

var (
	oldConfig = set(".yaml .yml .toml .ini .env .cfg .conf .tf .tfvars .hcl .properties .editorconfig .gitignore")
	oldHash   = set(".py .rb .pl .sh .bash .fish .nix .zsh .mk .cmake .r .jl .ex .exs .pp")
	oldSlash  = set(".go .rs .ts .tsx .js .jsx .mjs .cjs .c .h .cpp .hpp .cc .java .kt .swift .zig .cs .scala .dart .php .proto .qml")
	oldDash   = set(".lua .sql .hs .elm")
)

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

func oldClass(path string) Class {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)
	switch {
	case oldConfig[ext] || strings.HasPrefix(base, ".env"):
		return Config
	case oldHash[ext] || base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") || base == "Makefile":
		return Hash
	case oldSlash[ext]:
		return Slash
	case oldDash[ext]:
		return Dash
	}
	return None
}

func TestTheTableReproducesBothOldTables(t *testing.T) {
	var paths []string
	for _, m := range []map[string]bool{oldConfig, oldHash, oldSlash, oldDash} {
		for ext := range m {
			paths = append(paths, "/r/x"+ext, "/r/X"+strings.ToUpper(ext))
		}
	}
	for ext := range oldByExt {
		paths = append(paths, "/r/x"+ext)
	}
	for ext := range exts {
		paths = append(paths, "/r/x"+ext)
	}
	paths = append(paths, "/r/Makefile", "/r/GNUmakefile", "/r/Dockerfile", "/r/Dockerfile.dev", "/r/Dockerfile.yaml",
		"/r/.env", "/r/.env.local", "/r/x.unknown", "/r/README", "/r/makefile", "/r/.gitignore", "/r/x.ENV")
	for _, p := range paths {
		got := Of(p)
		if got.Grammar != oldGrammarFor(p) || got.Class != oldClass(p) {
			t.Errorf("%s: got %+v, the old tables say grammar %q class %v", p, got, oldGrammarFor(p), oldClass(p))
		}
	}
}

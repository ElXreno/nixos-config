package treesittertest

import (
	"os"
	"testing"
)

const (
	GrammarsEnv        = "AGENT_GUARD_GRAMMARS"
	RequireGrammarsEnv = "AGENT_GUARD_REQUIRE_GRAMMARS"
)

func Path(t testing.TB) string {
	t.Helper()
	path := os.Getenv(GrammarsEnv)
	if path != "" {
		return path
	}
	if os.Getenv(RequireGrammarsEnv) != "" {
		t.Fatal(GrammarsEnv + " is not set")
	}
	t.Skip(GrammarsEnv + " is not set")
	return ""
}

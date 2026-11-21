package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlashedProjectScopes(t *testing.T) {
	sb := newSandbox(t)
	code, _, errs := sb.run("Infra deploys go through the pipeline.", "put", "--key", "infra-pipeline", "--kind", "rule", "--scope", "project:Example-Org/infra", "--title", "Infra pipeline")
	if code != 0 {
		t.Fatalf("put: %d %s", code, errs)
	}
	code, out, _ := sb.run("", "get", "infra-pipeline")
	if code != 0 || !strings.Contains(out, "project:example-org/infra") {
		t.Errorf("get: %d %s", code, out)
	}
	code, _, errs = sb.run("A repo without a remote.", "put", "--key", "cludge-note", "--kind", "fact", "--scope", "project:~/tmp/royal-cludge-software")
	if code != 0 {
		t.Errorf("tilde project: %d %s", code, errs)
	}
	for _, bad := range []string{"project:/abs/x", "project:a//b", "project:a/", "host:a/b"} {
		code, _, errs := sb.run("Body of a bad scope record.", "put", "--key", "bad-scope", "--kind", "fact", "--scope", bad)
		if code == 0 || !strings.Contains(errs, "scope") {
			t.Errorf("%s accepted: %d %s", bad, code, errs)
		}
	}
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "feedback_infra.md"), []byte("---\nname: Infra rule\ndescription: Infra deploys\ntype: feedback\n---\nInfra deploys go through the pipeline, always.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs = sb.run("", "import-legacy", "--dir", dir, "--project", "Example-Org/infra")
	if code != 0 || !strings.Contains(out, "project:example-org/infra") {
		t.Fatalf("import-legacy: %d %q %q", code, out, errs)
	}
	code, out, _ = sb.run("", "search", "--scope", "project:example-org/infra", "pipeline")
	if code != 0 || !strings.Contains(out, "feedback_infra") || !strings.Contains(out, "infra-pipeline") {
		t.Errorf("search by the slashed scope: %d %s", code, out)
	}
	code, _, errs = sb.run("", "import-legacy", "--dir", dir, "--project", "/abs")
	if code == 0 || !strings.Contains(errs, "does not form a valid scope") {
		t.Errorf("leading slash: %d %s", code, errs)
	}
}

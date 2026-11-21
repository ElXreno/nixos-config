package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-guard/internal/call"
	"agent-guard/internal/treesitter"
	"agent-guard/internal/treesitter/treesittertest"
	"agent-guard/internal/verdict"
)

func testGrammars(t *testing.T) treesitter.Grammars {
	t.Helper()
	g, err := treesitter.FromFile(treesittertest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func onDisk(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCgoPreamble(t *testing.T) {
	in := write("/r/load.go", "package syntax\n\n// #cgo CFLAGS: -std=c11\n// #include <stdlib.h>\n// #include \"dl.h\"\nimport \"C\"\n")
	if got := Comments(in, Options{}).Worst(); got != verdict.Silent {
		t.Errorf("heuristic: a cgo preamble is not a comment wall, got %v", got)
	}
	if got := Comments(in, Options{Grammars: testGrammars(t)}).Worst(); got != verdict.Silent {
		t.Errorf("syntax: a cgo preamble is not a comment wall, got %v", got)
	}
}

func TestCommentsWithSyntax(t *testing.T) {
	g := testGrammars(t)
	rawString := onDisk(t, "tmpl.go", "package x\n\nvar tmpl = `\nPLACEHOLDER\n`\n")
	docAbove := onDisk(t, "drain.go", "package x\n\n// Drain empties the queue.\nfunc Drain() {}\n")
	story := "// We stopped the workers on 16.08 because the queue\n// had 5000 duplicates; 44% were the same job.\n// Now we drain them."

	cases := []struct {
		name string
		in   call.Change
		want verdict.Level
	}{
		{"python hashes inside a triple-quoted string", write("/r/x.py", "HELP = \"\"\"\n# Title\n# Section\n# More\n# Even more\n\"\"\"\nx = 1\n"), verdict.Silent},
		{"go slashes inside a raw string", write("/r/x.go", "package x\n\nvar tmpl = `\n// generated\n// do not edit\n// by hand\n`\n"), verdict.Silent},
		{"edit that lands inside a raw string on disk", edit(rawString, "PLACEHOLDER", "// one\n// two\n// three"), verdict.Silent},
		{"doc grows above an unchanged declaration", edit(docAbove, "// Drain empties the queue.", "// Drain empties the queue before the workers restart,\n// so the next batch starts from an empty table\n// and the counters line up with the new run.\n// It returns once the last item is acknowledged."), verdict.Silent},
		{"nix string holding c", write("/r/x.nix", "{ pkgs }:\npkgs.writeCBin \"w\" ''\n  #define _GNU_SOURCE\n  #include <stdio.h>\n  #include <stdlib.h>\n  int main(void) { return 0; }\n''\n"), verdict.Silent},
		{"go story block still blocked", write("/r/x.go", "package x\n\nfunc f() {\n\t"+strings.ReplaceAll(story, "\n", "\n\t")+"\n\twork(4)\n}\n"), verdict.Deny},
		{"yaml run block with a story still blocked", write("/r/ci.yaml", "steps:\n  - run: |\n      # first we wait for the socket, because the daemon\n      # races the unit on boot and loses the first\n      # connection otherwise\n      sleep 1\n"), verdict.Deny},
		{"nix string with a shell story still blocked", write("/r/x.nix", "{\n  script = ''\n    # first we wait for the socket, because the daemon\n    # races the unit on boot and loses the first\n    # connection otherwise\n    sleep 1\n  '';\n}\n"), verdict.Deny},
		{"terraform heredoc script with a story still blocked", write("/r/upgrade.tf", "resource \"terraform_data\" \"x\" {\n  provisioner \"local-exec\" {\n    command = <<-EOT\n      # Rebooting a node mid-resync wedges the peer's DRBD receiver, so never\n      # start a node while any resync is running; inconsistent replicas that\n      # are not syncing are ignored on purpose.\n      wait_no_resync\n    EOT\n  }\n}\n"), verdict.Deny},
		{"dockerfile heredoc with a story still blocked", write("/r/Dockerfile", "FROM alpine\nRUN <<EOF\n# first we wait for the socket, because the daemon\n# races the unit on boot and loses the first\n# connection otherwise\nsleep 1\nEOF\n"), verdict.Deny},
		{"sql story still blocked", write("/r/q.sql", "-- We stopped the workers because the queue\n-- had 5000 duplicates and 44% were the same job\n-- so now we drain them first\nSELECT 1;\n"), verdict.Deny},
		{"old_string missing on disk falls back", edit(rawString, "NOT THERE", story+"\nwork(4)"), verdict.Deny},
	}
	for _, tc := range cases {
		got := Comments(tc.in, Options{Grammars: g}).Worst()
		if got != tc.want {
			msg := ""
			if v := Comments(tc.in, Options{Grammars: g}); len(v) > 0 {
				msg = v[0].Message
			}
			t.Errorf("%s: want %v got %v\n  %s", tc.name, tc.want, got, msg)
		}
	}
}

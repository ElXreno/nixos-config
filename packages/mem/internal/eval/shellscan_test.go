package eval

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	shHome = "/home/u"
	shMem  = "/home/u/.claude/projects/-home-u-repo/memory"
)

func shAccept(p string) bool {
	return strings.HasSuffix(p, ".md") && filepath.Dir(p) == shMem
}

func render(scan ShellScan) []string {
	var out []string
	for _, e := range scan.Effects {
		rw := "R"
		if e.Write {
			rw = "W"
		}
		out = append(out, rw+":"+e.Op+":"+strings.TrimSuffix(filepath.Base(e.Path), ".md"))
	}
	return out
}

func TestScanShellCorpus(t *testing.T) {
	m := "~/.claude/projects/-home-u-repo/memory"
	abs := shMem
	cases := []struct {
		name string
		cmd  string
		want []string
	}{
		{"cat tilde", "cat " + m + "/a.md", []string{"R:cat:a"}},
		{"cat absolute", "cat " + abs + "/a.md", []string{"R:cat:a"}},
		{"cat HOME var", "cat $HOME/.claude/projects/-home-u-repo/memory/a.md | head -5", []string{"R:cat:a"}},
		{"head braces and quotes", `head -n 20 "${HOME}/.claude/projects/-home-u-repo/memory/a.md"`, []string{"R:head:a"}},
		{"tail two files", "tail -n+5 " + m + "/a.md " + m + "/b.md", []string{"R:tail:a", "R:tail:b"}},
		{"sed -n reads", "sed -n '1,40p' " + m + "/a.md", []string{"R:sed:a"}},
		{"sed -i writes", "sed -i 's/x/y/' " + m + "/a.md", []string{"W:sed-i:a"}},
		{"sed -ni writes", "sed -ni 's/x/y/p' " + m + "/a.md", []string{"W:sed-i:a"}},
		{"sed -i.bak writes", "sed -i.bak -e 's/x/y/' " + m + "/a.md", []string{"W:sed-i:a"}},
		{"sed -E reads", "sed -E 's/x+/y/' " + m + "/a.md", []string{"R:sed:a"}},
		{"awk reads", "awk '/x/' " + m + "/a.md", []string{"R:awk:a"}},
		{"awk inplace writes", "awk -i inplace '{print}' " + m + "/a.md", []string{"W:awk-inplace:a"}},
		{"less", "less " + m + "/a.md", []string{"R:less:a"}},
		{"bat", "bat -p " + m + "/a.md", []string{"R:bat:a"}},
		{"rg on a file", "rg -n foo " + m + "/a.md", []string{"R:rg:a"}},
		{"rg on a directory is no read", "rg -n foo " + m, nil},
		{"rg with glob is no read", "rg -l foo " + m + "/*.md", nil},
		{"rg --files directory", "rg --files " + m, nil},
		{"grep two files", "grep -n foo " + m + "/a.md " + m + "/b.md", []string{"R:grep:a", "R:grep:b"}},
		{"grep -r directory", "grep -rn foo " + m + "/", nil},
		{"brace expansion is no explicit file", "cat " + m + "/{a,b}.md", nil},
		{"variable unknown", `cat "$UNSET/a.md"`, nil},
		{"cd then relative", "cd " + m + " && cat a.md", []string{"R:cat:a"}},
		{"cd then dot slash", "cd " + abs + "; cat ./b.md", []string{"R:cat:b"}},
		{"relative without cwd", "cat a.md", nil},
		{"cd to dynamic path forgets cwd", "cd $X && cat a.md", nil},
		{"command substitution", `echo "$(cat ` + m + `/a.md)"`, []string{"R:cat:a"}},
		{"backticks", "echo `cat " + m + "/a.md`", []string{"R:cat:a"}},
		{"bash -c", "bash -c 'cat " + m + "/a.md'", []string{"R:cat:a"}},
		{"sh -lc", `sh -lc "cat ` + m + `/a.md"`, []string{"R:cat:a"}},
		{"ssh payload is remote", `ssh host "cat ` + m + `/a.md"`, nil},
		{"nix shell -c", "nix shell nixpkgs#bat -c bat " + m + "/a.md", []string{"R:bat:a"}},
		{"sudo -u", "sudo -u root cat " + m + "/a.md", []string{"R:cat:a"}},
		{"env prefix", "env FOO=1 cat " + m + "/a.md", []string{"R:cat:a"}},
		{"timeout", "timeout 5 cat " + m + "/a.md", []string{"R:cat:a"}},
		{"assignment prefix", "FOO=1 cat " + m + "/a.md", []string{"R:cat:a"}},
		{"variable path", "F=" + m + "/a.md; cat $F", []string{"R:cat:a"}},
		{"variable dir in quotes", `M=` + m + `; cat "$M/a.md"`, []string{"R:cat:a"}},
		{"index file is reported", "cat " + m + "/MEMORY.md " + m + "/a.md", []string{"R:cat:MEMORY", "R:cat:a"}},
		{"read feeds a pipe", "cat " + m + "/a.md | wc -l", []string{"R:cat:a"}},
		{"stdin redirect reads", "wc -l < " + m + "/a.md", []string{"R:redirect-in:a"}},
		{"output elsewhere", "cat " + m + "/a.md > /tmp/x", []string{"R:cat:a"}},
		{"comment", "# cat " + m + "/a.md", nil},
		{"trailing comment", "echo hi # cat " + m + "/a.md", nil},
		{"wc is no read", "wc -l " + m + "/a.md", nil},
		{"ls is no read", "ls -la " + m + "/a.md", nil},
		{"escaped space", `cat ` + m + `/a\ b.md`, []string{"R:cat:a b"}},
		{"other directory", "cat /home/u/.claude/projects/-home-u-other/memory/a.md", nil},
		{"redirect write", "echo foo > " + m + "/a.md", []string{"W:redirect:a"}},
		{"redirect append", "echo foo >> " + m + "/a.md", []string{"W:redirect:a"}},
		{"redirect clobber", "echo foo >| " + m + "/a.md", []string{"W:redirect:a"}},
		{"redirect both", "echo foo &> " + m + "/a.md", []string{"W:redirect:a"}},
		{"redirect fd", "echo foo 2> " + m + "/a.md", []string{"W:redirect:a"}},
		{"redirect fd dup", "echo foo 2>&1 >&2", nil},
		{"redirect to quoted var", `M=` + m + `; echo foo > "$M/a.md"`, []string{"W:redirect:a"}},
		{"heredoc to file", "cat <<EOF > " + m + "/a.md\nbody mentions " + m + "/b.md\nEOF\n", []string{"W:redirect:a"}},
		{"heredoc after redirect", "cat > " + m + "/a.md <<'EOF'\n" + m + "/b.md\nEOF\ncat " + m + "/c.md\n", []string{"W:redirect:a", "R:cat:c"}},
		{"heredoc strip tabs", "cat <<-EOF > " + m + "/a.md\n\tx\n\tEOF\ncat " + m + "/b.md", []string{"W:redirect:a", "R:cat:b"}},
		{"here-string", "cat <<< " + m + "/a.md", nil},
		{"tee", "echo x | tee -a " + m + "/a.md", []string{"W:tee:a"}},
		{"tee two", "echo x | tee " + m + "/a.md " + m + "/b.md", []string{"W:tee:a", "W:tee:b"}},
		{"python heredoc is no parsed write", "python3 - <<'EOF'\nopen('" + abs + "/a.md', 'w').write('x')\nEOF", nil},
		{"mv into", "mv /tmp/x.md " + m + "/a.md", []string{"W:mv:a"}},
		{"mv out", "mv " + m + "/a.md /tmp/x.md", []string{"W:mv:a"}},
		{"mv inside", "mv " + m + "/a.md " + m + "/b.md", []string{"W:mv:a", "W:mv:b"}},
		{"cp into directory", "cp /tmp/x.md " + m + "/", []string{"W:cp:x"}},
		{"cp into file", "cp /tmp/x.md " + m + "/a.md", []string{"W:cp:a"}},
		{"cp -t", "cp -t " + m + " /tmp/x.md /tmp/y.md", []string{"W:cp:x", "W:cp:y"}},
		{"cp from memory is no write", "cp " + m + "/a.md /tmp/a.md", nil},
		{"install", "install -m 644 /tmp/x.md " + m + "/a.md", []string{"W:install:a"}},
		{"rm", "rm -f " + m + "/a.md " + m + "/b.md", []string{"W:rm:a", "W:rm:b"}},
		{"rm glob", "rm " + m + "/old_*.md", nil},
		{"touch", "touch " + m + "/a.md", []string{"W:touch:a"}},
		{"truncate", "truncate -s 0 " + m + "/a.md", []string{"W:truncate:a"}},
		{"sponge", "jq . x.json | sponge " + m + "/a.md", []string{"W:sponge:a"}},
		{"perl -pi", "perl -pi -e 's/a/b/' " + m + "/a.md", []string{"W:perl-i:a"}},
		{"perl -i.bak", "perl -i.bak -pe 's/a/b/' " + m + "/a.md", []string{"W:perl-i:a"}},
		{"perl without -i", "perl -ne 'print' " + m + "/a.md", nil},
		{"dd of", "dd if=/dev/zero of=" + abs + "/a.md bs=1", []string{"W:dd:a"}},
		{"read then write in one line", "cat " + m + "/a.md > /tmp/x && sed -i 's/a/b/' " + m + "/a.md", []string{"R:cat:a", "W:sed-i:a"}},
		{"newline separated", "cd " + m + "\ncat a.md\nrm b.md\n", []string{"R:cat:a", "W:rm:b"}},
		{"line continuation", "cat \\\n" + m + "/a.md", []string{"R:cat:a"}},
		{"subshell", "(cd " + m + " && cat a.md)", []string{"R:cat:a"}},
		{"unterminated quote", "cat '" + abs + "/a.md", []string{"R:cat:a"}},
		{"empty", "", nil},
	}
	for _, c := range cases {
		got := render(ScanShell(c.cmd, shHome, shAccept))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %q\n got %v\nwant %v", c.name, c.cmd, got, c.want)
		}
	}
}

func TestScanShellNamesCommands(t *testing.T) {
	scan := ScanShell("cd /tmp && python3 -c 'print(1)' | tee out.txt; sudo perl -e 1", shHome, shAccept)
	if want := []string{"cd", "python3", "tee", "perl"}; !slices.Equal(scan.Commands, want) {
		t.Errorf("commands %v, want %v", scan.Commands, want)
	}
	if !isInterpreter("python3") || !isInterpreter("python3.12") || isInterpreter("cat") {
		t.Error("interpreter detection")
	}
}

func TestScanShellSurvivesJunk(t *testing.T) {
	for _, junk := range []string{"$(", "$((1+", "`", "'", `"`, "<<", "<<EOF", "${", "${A", ">", "&>", "cat <(", "a | | b", "\\", "$", "$'", "~", "~x/y", ")))", "{ cat; }", "cat <<EOF\nnever closed"} {
		_ = ScanShell(junk, shHome, shAccept)
	}
	deep := strings.Repeat("$(", 40) + "cat " + shMem + "/a.md" + strings.Repeat(")", 40)
	if got := render(ScanShell(deep, shHome, shAccept)); len(got) > 1 {
		t.Errorf("deep nesting %v", got)
	}
}

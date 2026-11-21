package shell

import (
	"strings"
	"testing"
)

func heads(a *Analysis) []string {
	var out []string
	for _, c := range a.Commands {
		out = append(out, c.Head())
	}
	return out
}

func has(a *Analysis, head string) bool {
	for _, h := range heads(a) {
		if h == head {
			return true
		}
	}
	return false
}

func background(a *Analysis) bool {
	for _, c := range a.Commands {
		if c.Background {
			return true
		}
	}
	return false
}

func detached(a *Analysis) bool {
	for _, c := range a.Commands {
		if c.Detached {
			return true
		}
	}
	return false
}

func TestPayloadsReachTheRealProgram(t *testing.T) {
	cases := []struct {
		src, head string
	}{
		{`bwrap --ro-bind / / bash -c 'pkill -f x'`, "pkill"},
		{`bwrap --ro-bind / / -- bash -c 'pkill -f x'`, "pkill"},
		{`bwrap --unshare-all --bind /a /b --setenv K V --dev /dev pkill -f x`, "pkill"},
		{`env -S 'pkill -f x'`, "pkill"},
		{`env --split-string='pkill -f x'`, "pkill"},
		{`env -i -S 'pkill -f x'`, "pkill"},
		{`flock /tmp/l -c 'pkill -f x'`, "pkill"},
		{`flock --timeout 5 /tmp/l --command 'pkill -f x'`, "pkill"},
		{`flock -w 5 /tmp/l pkill -f x`, "pkill"},
		{`watch -n 5 'pkill x'`, "pkill"},
		{`watch -n 5 pkill x`, "pkill"},
		{`watch -x pkill x`, "pkill"},
		{`sudo -s 'pkill -f x'`, "pkill"},
		{`sudo -i pkill -f x`, "pkill"},
		{`runuser -l u -c 'pkill -f x'`, "pkill"},
		{`runuser -u u -- pkill -f x`, "pkill"},
		{`su - root -c 'pkill -f x'`, "pkill"},
		{`script -q -c 'pkill -f x' /dev/null`, "pkill"},
		{`xargs -i pkill {}`, "pkill"},
		{`xargs -l pkill`, "pkill"},
		{`xargs -I{} pkill {}`, "pkill"},
		{`xargs -n 1 pkill`, "pkill"},
		{`parallel -j 4 pkill`, "pkill"},
		{`exec -a name pkill -f x`, "pkill"},
		{`bash -O extglob -c 'pkill -f x'`, "pkill"},
		{`bash --rcfile /x -c 'pkill -f x'`, "pkill"},
		{`bash --init-file /x -c 'pkill -f x'`, "pkill"},
		{`docker run --entrypoint sh img -c 'pkill -f x'`, "pkill"},
		{`docker run --entrypoint=sh img -c 'pkill -f x'`, "pkill"},
	}
	for _, c := range cases {
		if a := Analyze(c.src); !has(a, c.head) {
			t.Errorf("%s: want a %s command, got heads %q", c.src, c.head, heads(a))
		}
	}
}

func TestShellStringCarriersBackground(t *testing.T) {
	for _, src := range []string{
		`flock /tmp/l -c 'sleep 999 &'`,
		`runuser -l u -c 'sleep 999 &'`,
		`watch -n 5 'sleep 999 &'`,
		`env -S 'sleep 999 &'`,
	} {
		if !background(Analyze(src)) {
			t.Errorf("%s: the payload's & must be seen", src)
		}
	}
}

func TestDetachedThroughWrappers(t *testing.T) {
	for _, src := range []string{`nix shell p -c nohup cmd`, `xargs nohup x`, `nohup x`, `sudo setsid x`, `x & disown`, `nohup`} {
		if !detached(Analyze(src)) {
			t.Errorf("%s: want a detached command", src)
		}
	}
	for _, src := range []string{`nohupx`, `echo nohup`} {
		if detached(Analyze(src)) {
			t.Errorf("%s: nothing is detached", src)
		}
	}
}

func TestLookupsDoNotRun(t *testing.T) {
	for _, src := range []string{`command -v pkill`, `command -V pkill`, `command -pv pkill`} {
		if has(Analyze(src), "pkill") {
			t.Errorf("%s only looks pkill up", src)
		}
	}
	if !has(Analyze(`command pkill -f x`), "pkill") {
		t.Error("command without -v runs the program")
	}
}

func nest(cmd string, levels int) string {
	for range levels {
		cmd = "sh -c '" + strings.ReplaceAll(cmd, "'", `'\''`) + "'"
	}
	return cmd
}

func TestDepthTruncationIsRecorded(t *testing.T) {
	deep := Analyze(nest("pkill x", MaxDepth+1))
	if len(deep.Unparsed) != 1 || deep.Unparsed[0].Reason != ReasonDepth || deep.Unparsed[0].Text != "pkill x" {
		t.Errorf("the payload past the limit is recorded, got %+v", deep.Unparsed)
	}
	if has(deep, "pkill") {
		t.Error("the truncated payload is not analysed")
	}
	shallow := Analyze(nest("pkill x", MaxDepth))
	if len(shallow.Unparsed) != 0 || !has(shallow, "pkill") {
		t.Errorf("nesting up to the limit is analysed, got %+v", shallow.Unparsed)
	}
}

func TestSyntheticCommandsCarryTheirOrigin(t *testing.T) {
	a := Analyze(`(sleep 5) &`)
	var compound *Command
	for i, c := range a.Commands {
		if c.Origin == Compound {
			compound = &a.Commands[i]
		}
	}
	if compound == nil || compound.Argv != nil || !compound.Background {
		t.Errorf("a backgrounded compound is a synthetic command with no argv: %+v", a.Commands)
	}
	f := Analyze(`for x in a b; nohup x &; end`)
	if len(f.Unparsed) == 0 || f.Unparsed[0].Reason != ReasonParse || f.Unparsed[0].Err == nil {
		t.Fatalf("fish does not parse: %+v", f.Unparsed)
	}
	sawFallback := false
	for _, c := range f.Commands {
		if c.Origin == Fallback && c.Background && c.Argv != nil {
			t.Errorf("the fallback & is synthetic: %+v", c)
		}
		sawFallback = sawFallback || c.Origin == Fallback
	}
	if !sawFallback || !background(f) || !detached(f) {
		t.Errorf("the fallback still reports & and nohup: %+v", f.Commands)
	}
}

func TestUnclosedHeredocIsForgiven(t *testing.T) {
	a := Analyze("python3 - <<'PY' || python - <<'PY'\nprint(1)\n")
	if len(a.Unparsed) != 0 {
		t.Errorf("an unclosed heredoc is closed at end of input, got %+v", a.Unparsed)
	}
}

func TestQuotedHeredocKeepsBackslashes(t *testing.T) {
	a := Analyze("bash <<'EOF'\necho a\\&b\nEOF")
	if background(a) {
		t.Errorf("a quoted heredoc keeps \\& literal, nothing runs in the background: %+v", a.Commands)
	}
	b := Analyze("bash <<EOF\necho a\\&b\nEOF")
	if background(b) {
		t.Errorf("an unquoted heredoc keeps \\& too, bash only unescapes \\$ \\` \\\\ and newline: %+v", b.Commands)
	}
	c := Analyze("bash <<EOF\nsleep 9 &\nEOF")
	if !background(c) {
		t.Error("a real & in a heredoc is still seen")
	}
}

func TestStdinScripts(t *testing.T) {
	cases := map[string]string{
		`bash -s <<< 'rm -rf /x'`:                 "rm -rf /x\n",
		`echo 'rm -rf /x' | sh`:                   "rm -rf /x\n",
		`echo -n rm -rf /x | sh`:                  "rm -rf /x\n",
		`echo -e 'cd /a\nrm x' | sh`:              "cd /a\nrm x\n",
		`echo 'cd /a\nrm x' | sh`:                 "cd /a\\nrm x\n",
		`printf 'rm %s\n' a b | sh`:               "rm a\nrm b\n",
		`printf '%s %%\n' x | sh`:                 "x %\n",
		`printf -- '%5.2s|%-*s\n' ab 3 c | sh`:    "ab|c\n",
		`printf '%b' 'rm\tx' | sh`:                "rm\tx",
		`X=/srv; echo "rm -rf $X" | sudo bash`:    "rm -rf /srv\n",
		`echo 'rm -rf /x' | sh | cat`:             "rm -rf /x\n",
		`printf -v v 'rm -rf /x'; echo $v | grep`: "$v\n",
	}
	for src, want := range cases {
		a := Analyze(src)
		got := ""
		for _, c := range a.Commands {
			if c.Stdin != "" {
				got = c.Stdin
			}
		}
		if got != want {
			t.Errorf("%s: stdin %q, want %q", src, got, want)
		}
	}
	if a := Analyze(`echo 'rm -rf /x' | sh`); len(a.Commands) < 3 || a.Commands[len(a.Commands)-1].Head() != "rm" {
		t.Errorf("the piped script is analysed: %+v", a.Commands)
	}
	for _, src := range []string{
		`echo 'rm -rf /x' | grep rm`,
		`echo 'rm -rf /x' | bash ./script.sh`,
		`echo 'rm -rf /x' | bash -c 'cat'`,
		`printf '%s' 'select 1; rm' | ssh host bash -c "'sudo -u postgres psql'"`,
	} {
		for _, c := range Analyze(src).Commands {
			if c.Head() == "rm" {
				t.Errorf("%s: piped data is no script here: %+v", src, c)
			}
		}
	}
	for _, src := range []string{
		`echo 'rm -rf /x' | bash -s arg`,
		`echo 'rm -rf /x' | bash -euo pipefail`,
		`echo 'rm -rf /x' | ssh host`,
		`echo 'rm -rf /x' | ssh host 'bash -s'`,
		`echo 'rm -rf /x' | docker exec -i ctr sh`,
		`echo 'rm -rf /x' | su - user`,
	} {
		seen := false
		for _, c := range Analyze(src).Commands {
			seen = seen || c.Head() == "rm"
		}
		if !seen {
			t.Errorf("%s: the piped script is analysed", src)
		}
	}
}

func TestNoAITypographyInSources(t *testing.T) {
	for _, s := range []string{Analyze("x").Source} {
		if strings.ContainsAny(s, "…—") {
			t.Error(s)
		}
	}
}

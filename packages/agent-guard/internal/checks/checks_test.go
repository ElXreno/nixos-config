package checks

import (
	"strings"
	"testing"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

var testSettings = &Settings{
	Home:           "/home/test",
	ShellReadRoots: []string{"/proc", "/sys", "/dev", "/etc", "/usr", "/nix/store", "/run/current-system", "/home/test/.claude"},
	ProtectedRepos: []string{"github.com/ElXreno/nixos-config"},
}

var matrix = []struct {
	check string
	want  string
	cmd   string
}{
	// background: the original report, ssh payloads
	{"background", "deny", `ssh root@localhost "blabla &"`},
	{"background", "deny", `ssh host 'nohup x &'`},
	{"background", "deny", `ssh -p 22 -o StrictHostKeyChecking=no root@host bash -c "sleep 5 &"`},
	{"background", "deny", `timeout 30 env FOO=1 ssh host "long-job &"`},
	{"background", "silent", `ssh host cmd arg "a & b"`},
	{"background", "deny", `ssh host "curl http://x?a=1&b=2"`},
	{"background", "silent", `ssh host "curl 'http://x?a=1&b=2'"`},
	{"background", "deny", `sudo bash -c "daemon &"`},
	{"background", "deny", `nix shell nixpkgs#foo -c bash -c "server &"`},
	{"background", "deny", `kubectl -n ns exec pod -- sh -c "tail -f /x &"`},
	{"background", "deny", `docker exec -it ctr bash -c "svc start &"`},
	{"background", "deny", "ssh host bash -s <<'EOF'\nfoo &\nEOF"},
	{"background", "silent", "cat > /tmp/x <<EOF\nnot a & command\nEOF"},
	{"background", "deny", `echo $(nohup x)`},
	{"background", "silent", `echo "a & b"`},
	{"background", "silent", `cmd 2>&1 | tee log`},
	{"background", "silent", `cmd &>/tmp/log`},
	{"background", "silent", `a && b || c`},
	{"background", "silent", `ssh host "systemctl status foo"`},
	{"background", "deny", `python3 -c "import subprocess; subprocess.run(\"svc &\", shell=True)"`},
	{"background", "deny", "python3 - <<'PY'\nimport os\nos.system(\"svc &\")\nPY"},
	{"background", "deny", `nohup make`},
	{"background", "silent", `ssh host "printf %s \"x&y\""`},
	{"background", "deny", `(sleep 5; make) &`},
	{"background", "deny", `{ a; b; } &`},

	// pkill and killall, nested
	{"pkill", "deny", `pkill -f foo`},
	{"pkill", "deny", `killall firefox`},
	{"pkill", "deny", `sudo pkill x`},
	{"pkill", "deny", `ssh root@host "pkill -f nginx"`},
	{"pkill", "deny", `ssh host killall x`},
	{"pkill", "deny", `python3 -c "import subprocess; subprocess.run(\"pkill x\", shell=True)"`},
	{"pkill", "deny", `fd -x sh -c "pkill -f {}"`},
	{"pkill", "silent", `echo "pkill is blocked"`},
	{"pkill", "silent", `pgrep -a foo`},
	{"pkill", "silent", `kill 1234`},

	// sleep
	{"sleep", "silent", `sleep 30`},
	{"sleep", "advise", `sleep 45`},
	{"sleep", "deny", `sleep 120`},
	{"sleep", "deny", `ssh host "sleep 300; systemctl restart x"`},
	{"sleep", "deny", `sudo bash -c "sleep 600"`},
	{"sleep", "deny", `echo $(sleep 200)`},
	{"sleep", "silent", `echo "sleep 999 is too long"`},
	{"sleep", "deny", `sleep 5m`},
	{"sleep", "silent", `timeout 300 nix build`},

	// shell-tool: cat is advisory, read-only roots exempt, remote payloads skipped
	{"shell-tool", "advise", `cat modules/x.nix`},
	{"shell-tool", "advise", `ls dir; echo ===; cat dir/meson.build`},
	{"shell-tool", "silent", `cat /etc/hosts`},
	{"shell-tool", "silent", `ls /nix/store/abc-source/protocol/; cat /nix/store/abc-source/protocol/meson.build`},
	{"shell-tool", "silent", `ssh host "cat /etc/hosts"`},
	{"shell-tool", "silent", `ssh root@keenetic "grep x /etc/passwd"`},
	{"shell-tool", "silent", `kubectl -n ns exec pod -- sh -c "cat /etc/hosts"`},
	{"shell-tool", "silent", `docker exec ctr cat /etc/os-release`},
	{"shell-tool", "silent", `sudo bash -c "cat /etc/shadow"`},
	{"shell-tool", "advise", `nix shell nixpkgs#foo -c bash -c "cat file.nix"`},
	{"shell-tool", "advise", `sudo bash -c "cat /var/lib/x/state"`},
	{"shell-tool", "silent", `cat /proc/cpuinfo`},
	{"shell-tool", "silent", `cat ~/.claude/settings.json`},
	{"shell-tool", "silent", `cat file | head`},
	{"shell-tool", "deny", `find . -name "*.nix"`},
	{"shell-tool", "silent", `ssh host "find / -name x"`},
	{"shell-tool", "deny", `grep -rn foo src/`},
	{"shell-tool", "silent", `ps aux | grep foo`},
	{"shell-tool", "silent", `ssh host "find / -name x" && cat /proc/version`},
	{"shell-tool", "deny", `echo "n: $(grep -c pattern file.txt)"`},

	// git scope is denied, destructive git asks
	{"git-scope", "deny", `git add -A`},
	{"git-scope", "deny", `git add .`},
	{"git-scope", "deny", `git commit -am "x"`},
	{"git-scope", "silent", `git add modules/x.nix`},
	{"git-scope", "silent", `git commit -m "x" -- a b`},
	{"git-scope", "deny", `ssh host "cd /repo && git add -A"`},
	{"git-scope", "silent", `git add ./src`},
	{"git-scope", "silent", `git commit -m "add -a flag"`},
	{"git-destructive", "ask", `git reset --hard HEAD~1`},
	{"git-destructive", "ask", `git checkout -- .`},
	{"git-destructive", "ask", `git stash`},
	{"git-destructive", "ask", `git stash push -m x`},
	{"git-destructive", "silent", `git stash list`},
	{"git-destructive", "silent", `git stash pop`},
	{"git-destructive", "ask", `git clean -fdx`},
	{"git-destructive", "silent", `git reset --soft HEAD~1`},
	{"git-destructive", "silent", `git checkout -- modules/x.nix`},
	{"git-destructive", "ask", `sudo bash -c "cd /repo && git reset --hard"`},
	{"git-destructive", "ask", `git -C /repo reset --hard origin/main`},

	{"git-push", "ask", `git push git@github.com:ElXreno/nixos-config.git main`},
	{"git-push", "ask", `git push https://github.com/elxreno/nixos-config HEAD:main`},
	{"git-push", "ask", `ssh host "cd nixos-config && git push"`},
	{"git-push", "silent", `git push https://github.com/ElXreno/other main`},
	{"git-push", "silent", `git push --dry-run https://github.com/ElXreno/nixos-config`},
	{"git-push", "silent", `gh repo sync ElXreno/nixos-config`},
	{"git-push", "silent", `echo "git push origin main"`},

	// rg -r and nix store roots, nested too
	{"rg-replace", "deny", `rg -rn foo .`},
	{"rg-replace", "silent", `rg -n foo .`},
	{"rg-replace", "deny", `ssh host "rg -rl foo /etc"`},
	{"rg-replace", "silent", `rg -n "a -r b" .`},
	{"rg-replace", "silent", `rg -o 'X (\w+)' --replace '$1' f`},
	{"rg-replace", "silent", `rg --replace='$1' x f`},
	{"nix-store-root", "deny", `ls /nix/store`},
	{"nix-store-root", "deny", `ssh host "du -sh /nix/store"`},
	{"nix-store-root", "silent", `ls /nix/store/abc-foo/bin`},
	{"nix-store-root", "deny", `fd foo /nix/store`},
	{"nix-store-root", "silent", `nix path-info nixpkgs#hello`},
	{"nix-store-root", "deny", `f=$(find /nix/store -maxdepth 1 -name '*foo*' | head -1)`},

	// carriers and dialects found by the corpus replay
	{"background", "deny", `adb shell 'logcat -d > /sdcard/x.txt &'`},
	{"shell-tool", "silent", `adb shell 'find /vendor -name "*.ko"'`},
	{"sleep", "deny", `ssh root@host bash -c "' mount /x || exit 1; sleep 100 '"`},
	{"nix-store-root", "deny", "ssh root@host 'bash -s' <<'EOF'\nls /nix/store\nEOF"},
	{"sleep", "deny", `for i in (seq 1 10); sleep 120; set st (ssh boba 'true'); echo $st; end`},
	{"background", "deny", `podman run --rm --entrypoint sh img -c 'svc &'`},
	{"shell-tool", "silent", `grep -e a -e b`},
	{"shell-tool", "deny", `grep -A 3 foo file.txt`},
	{"shell-tool", "silent", `grep --include=*.go -c foo`},
	{"background", "deny", "ssh root@host bash -c \"'\nsleep 60 &\nwait\n'\""},

	// variables assigned in the same command are substituted
	{"sleep", "deny", `N=100; sleep $N`},
	{"sleep", "deny", "export WAIT=5m\nsleep $WAIT"},
	{"sleep", "silent", `N=$(cat n); sleep $N`},
	{"nix-store-root", "deny", `S=/nix/store; ls $S`},
	{"nix-store-root", "deny", `S=/nix; du -sh ${S}/store`},
	{"pkill", "deny", `K=pkill; $K -f foo`},

	// quoting and structure that must not trip anything
	{"background", "silent", `echo "it's & ok"`},
	{"background", "silent", `awk '$1 == "a" && $2 == "b"' file`},
	{"pkill", "silent", "cat <<'EOF'\npkill inside data\nEOF"},
	{"background", "silent", `x=$((3 & 5)); echo $x`},
	{"shell-tool", "silent", `echo cat`},
	{"sleep", "silent", `echo "sleep"`},
	{"background", "silent", `export FOO="a & b"`},
}

func TestMatrix(t *testing.T) {
	for _, tc := range matrix {
		a := shell.Analyze(tc.cmd)
		got := runOne(tc.check, a, testSettings).Worst().String()
		if got != tc.want {
			t.Errorf("%s: want %s got %s for %q", tc.check, tc.want, got, tc.cmd)
		}
	}
}

func TestInPlaceEditsDoNotPrompt(t *testing.T) {
	for _, cmd := range []string{
		`sed -i "s/a/b/" modules/x.nix`,
		`perl -pi -e "s/a/b/" x.go`,
		`sd "foo" "bar" src/main.rs`,
		`jq . x.json | sponge x.json`,
		"python3 - <<'PY'\nopen('notes.md', 'w').write('x')\nPY",
		`D=/mnt/scratch/claude-code/x; cd $D && python3 -c "open('out.json','w').write('{}')"`,
	} {
		if v := Evaluate(shell.Analyze(cmd), Context{}, testSettings); v.Worst() >= verdict.Ask {
			t.Errorf("%q must run without a dialog, got %v: %v", cmd, v.Worst(), v)
		}
	}
}

func TestAnalyzerBypassesAreClosed(t *testing.T) {
	nested := "pkill x"
	for range shell.MaxDepth + 1 {
		nested = "sh -c '" + strings.ReplaceAll(nested, "'", `'\''`) + "'"
	}
	cases := []struct {
		cmd, want string
	}{
		{`bwrap --ro-bind / / bash -c 'pkill -f x'`, "deny"},
		{`env -S 'pkill -f x'`, "deny"},
		{`flock /tmp/l -c 'sleep 999 &'`, "deny"},
		{`watch -n 5 'pkill x'`, "deny"},
		{`xargs -i pkill {}`, "deny"},
		{`nix shell p -c nohup cmd`, "deny"},
		{`runuser -l u -c 'sleep 999 &'`, "deny"},
		{`command -v pkill`, "silent"},
		{nested, "advise"},
	}
	for _, c := range cases {
		got := Evaluate(shell.Analyze(c.cmd), Context{}, testSettings).Worst().String()
		if got != c.want {
			t.Errorf("%s: want %s got %s", c.cmd, c.want, got)
		}
	}
	v := Evaluate(shell.Analyze(nested), Context{}, testSettings)
	if len(v) != 1 || v[0].Check != Parser || !strings.Contains(v[0].Message, "nests interpreters more than 4 levels deep") {
		t.Errorf("depth truncation surfaces as the parser advice: %+v", v)
	}
}

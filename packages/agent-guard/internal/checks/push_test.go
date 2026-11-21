package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

const protectedURL = "git@github.com:ElXreno/nixos-config.git"

func isolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_DIR", "")
	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command(gitProgram, append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func repo(t *testing.T, dir string, commit bool, config ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	for i := 0; i+1 < len(config); i += 2 {
		git(t, dir, "config", "--add", config[i], config[i+1])
	}
	if commit {
		git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
	}
}

type pushFixture struct {
	root string
	cfg  *Settings
}

func newPushFixture(t *testing.T) pushFixture {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	other := "https://github.com/ElXreno/other.git"
	repo(t, filepath.Join(root, "prot"), false, "remote.origin.url", protectedURL)
	if err := os.MkdirAll(filepath.Join(root, "prot", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo(t, filepath.Join(root, "free"), true, "remote.origin.url", other)
	repo(t, filepath.Join(root, "pushurl"), true, "remote.origin.url", other, "remote.origin.pushurl", "https://github.com/ElXreno/nixos-config")
	repo(t, filepath.Join(root, "mirror"), true, "remote.origin.url", protectedURL, "remote.origin.pushurl", "https://github.com/ElXreno/mirror.git")
	repo(t, filepath.Join(root, "pushremote"), true, "remote.origin.url", other, "remote.up.url", "https://github.com/elxreno/NixOS-Config/", "branch.main.pushRemote", "up")
	repo(t, filepath.Join(root, "upstream"), true, "remote.origin.url", other, "remote.up.url", "ssh://git@github.com:22/ElXreno/nixos-config.git", "branch.main.remote", "up", "branch.main.merge", "refs/heads/main")
	repo(t, filepath.Join(root, "alias"), true, "remote.origin.url", other, "url.git@github.com:ElXreno/.insteadOf", "mine:")
	repo(t, filepath.Join(root, "lone"), true, "remote.up.url", protectedURL)
	if err := os.MkdirAll(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	return pushFixture{root: root, cfg: &Settings{Home: filepath.Join(root, "plain"), ProtectedRepos: []string{"github.com/ElXreno/nixos-config"}}}
}

func (f pushFixture) push(cmd, cwd string) *verdict.Verdict {
	r := strings.NewReplacer("PROT", filepath.Join(f.root, "prot"), "FREE", filepath.Join(f.root, "free"), "ROOT", f.root)
	cmd = r.Replace(cmd)
	var dir string
	if cwd != "" {
		dir = filepath.Join(f.root, cwd)
	}
	for _, v := range Evaluate(shell.Analyze(cmd), Context{Cwd: dir}, f.cfg) {
		if v.Check == GitPush {
			return &v
		}
	}
	return nil
}

func TestGitPush(t *testing.T) {
	f := newPushFixture(t)
	cases := []struct {
		name, cwd, cmd string
		want           verdict.Level
	}{
		{"cwd in the protected checkout", "prot", `git push`, verdict.Ask},
		{"cwd in a subdirectory of it", "prot/sub", `git push origin HEAD`, verdict.Ask},
		{"named remote with upstream", "prot", `git push -u origin main`, verdict.Ask},
		{"force with lease", "prot", `git push --force-with-lease origin main`, verdict.Ask},
		{"branch delete", "prot", `git push origin --delete old`, verdict.Ask},
		{"wrapped in timeout", "prot", `timeout 60 git push`, verdict.Ask},

		{"cd earlier in the line", "free", `cd PROT && git push`, verdict.Ask},
		{"relative cd", "", `cd ROOT/prot; git push`, verdict.Ask},
		{"pushd earlier in the line", "free", `pushd PROT && git push && popd`, verdict.Ask},
		{"popd back to the unprotected cwd", "free", `pushd PROT >/dev/null; popd; git push`, verdict.Silent},
		{"cd inside a payload does not leak out", "free", `bash -c 'cd PROT'; git push`, verdict.Silent},

		{"-C absolute", "free", `git -C PROT push`, verdict.Ask},
		{"-C absolute without any cwd", "", `git -C PROT push`, verdict.Ask},
		{"-C relative chain", "prot", `git -C .. -C prot/sub push`, verdict.Ask},
		{"-C into an unprotected checkout", "prot", `git -C FREE push`, verdict.Silent},
		{"--git-dir=", "free", `git --git-dir=PROT/.git push`, verdict.Ask},
		{"--git-dir separate", "free", `git --git-dir PROT/.git push origin main`, verdict.Ask},
		{"GIT_DIR prefix", "free", `GIT_DIR=PROT/.git git push`, verdict.Ask},
		{"exported GIT_DIR", "free", `export GIT_DIR=PROT/.git; git push`, verdict.Ask},
		{"env GIT_DIR", "free", `env GIT_DIR=PROT/.git git push`, verdict.Ask},
		{"env -C", "free", `env -C PROT git push`, verdict.Ask},
		{"-c pushurl override", "free", `git -c remote.origin.pushurl=` + protectedURL + ` push`, verdict.Ask},

		{"sh -c payload", "free", `sh -c "cd PROT && git push"`, verdict.Ask},
		{"sudo bash -c payload", "free", `sudo -u elx bash -c 'git -C PROT push origin main'`, verdict.Ask},
		{"heredoc to bash", "free", "bash <<'EOF'\ncd PROT\ngit push\nEOF", verdict.Ask},
		{"nix develop -c", "free", `nix develop -c git -C PROT push`, verdict.Ask},
		{"ssh payload cannot be read", "free", `ssh host 'cd nixos-config && git push'`, verdict.Ask},
		{"container payload cannot be read", "free", `docker exec ctr git push`, verdict.Ask},
		{"ssh payload with an unprotected URL", "prot", `ssh host git push https://github.com/ElXreno/other.git main`, verdict.Silent},
		{"ssh payload with a protected URL", "free", `ssh host "git push https://github.com/ElXreno/nixos-config main"`, verdict.Ask},

		{"https URL", "free", `git push https://github.com/ElXreno/nixos-config.git HEAD:main`, verdict.Ask},
		{"ssh URL with a port", "free", `git push ssh://git@github.com:22/ElXreno/nixos-config main`, verdict.Ask},
		{"scp URL in another case", "free", `git push git@github.com:elxreno/NixOS-Config main`, verdict.Ask},
		{"--repo", "free", `git push --repo=https://github.com/ElXreno/nixos-config`, verdict.Ask},
		{"insteadOf alias", "alias", `git push mine:nixos-config main`, verdict.Ask},
		{"URL without any cwd", "", `git push ` + protectedURL, verdict.Ask},
		{"unprotected URL from the protected checkout", "prot", `git push https://github.com/ElXreno/other.git main`, verdict.Silent},
		{"local path from the protected checkout", "prot", `git push ../free main`, verdict.Silent},

		{"pushurl wins over the fetch url", "pushurl", `git push`, verdict.Ask},
		{"protected fetch url, pushurl elsewhere", "mirror", `git push`, verdict.Silent},
		{"branch pushRemote", "pushremote", `git push`, verdict.Ask},
		{"branch upstream remote", "upstream", `git push`, verdict.Ask},
		{"the only remote is the default", "lone", `git push`, verdict.Ask},
		{"explicit origin beside a protected pushRemote", "pushremote", `git push origin main`, verdict.Silent},

		{"unprotected checkout", "free", `git push`, verdict.Silent},
		{"unprotected named remote", "free", `git push -u origin feature`, verdict.Silent},
		{"not a repository", "plain", `git push`, verdict.Silent},
		{"missing directory", "free", `git -C ROOT/missing push`, verdict.Silent},
		{"no cwd and no URL", "", `git push`, verdict.Silent},

		{"variable remote inside the protected checkout", "prot", `git push $AG_PUSH_TARGET_UNSET main`, verdict.Ask},
		{"substituted remote inside the protected checkout", "prot", `git push "$(git remote | head -1)" main`, verdict.Ask},
		{"unknown remote inside the protected checkout", "prot", `git push nosuchremote main`, verdict.Ask},
		{"variable -C from the protected checkout", "prot", `git -C "$AG_PUSH_DIR_UNSET" push`, verdict.Ask},
		{"variable GIT_DIR from the protected checkout", "prot", `GIT_DIR=$(cat where) git push`, verdict.Ask},
		{"variable remote outside it", "free", `git push $AG_PUSH_TARGET_UNSET main`, verdict.Ask},
		{"unknown remote outside it", "free", `git push nosuchremote`, verdict.Silent},
		{"variable -C outside it", "free", `git -C "$AG_PUSH_DIR_UNSET" push`, verdict.Ask},
		{"variable -C outside any checkout", "plain", `git -C "$AG_PUSH_DIR_UNSET" push`, verdict.Ask},
		{"variable -C without any cwd", "", `git -C "$AG_PUSH_DIR_UNSET" push`, verdict.Ask},
		{"variable -C with an unprotected URL", "free", `git -C "$AG_PUSH_DIR_UNSET" push https://github.com/ElXreno/other.git`, verdict.Silent},
		{"assigned -C stays exact", "plain", `D=FREE; git -C $D push`, verdict.Silent},
		{"for loop over -C", "free", `for d in PROT; do git -C $d push; done`, verdict.Ask},
		{"for loop over cd", "free", `for d in PROT; do cd $d; git push; done`, verdict.Ask},
		{"cd to a variable then a relative cd", "free", `cd $AG_PUSH_DIR_UNSET && cd sub && git push`, verdict.Ask},
		{"cd to a variable then an absolute cd", "plain", `cd $AG_PUSH_DIR_UNSET; cd FREE; git push`, verdict.Silent},
		{"cd - is not followed", "free", `cd PROT; cd FREE; cd -; git push`, verdict.Ask},
		{"pushd of a variable then popd", "free", `pushd $AG_PUSH_DIR_UNSET; popd; git push`, verdict.Silent},
		{"xargs -I placeholder", "free", `echo PROT | xargs -I{} git -C {} push`, verdict.Ask},
		{"xargs appends the destination", "free", `echo ` + protectedURL + ` | xargs git push`, verdict.Ask},
		{"xargs with an unprotected URL", "free", `echo main | xargs git push https://github.com/ElXreno/other.git`, verdict.Silent},
		{"parallel placeholder", "free", `parallel git -C {} push ::: PROT`, verdict.Ask},

		{"--dry-run", "prot", `git push --dry-run`, verdict.Silent},
		{"-n inside a cluster", "prot", `git push -fn origin main`, verdict.Silent},
		{"dry run of a protected URL", "free", `git push --dry-run ` + protectedURL, verdict.Silent},
		{"an attached -o value is no dry run", "prot", `git push -onotify origin main`, verdict.Ask},
		{"a push option value", "prot", `git push -o ci.skip origin main`, verdict.Ask},
		{"abbreviated --dry is not trusted", "prot", `git push --dry origin main`, verdict.Ask},
		{"--no-dry-run cancels --dry-run", "prot", `git push --dry-run --no-dry-run`, verdict.Ask},
		{"--no-dry-run cancels -n", "free", `git -C PROT push -n --no-dry-run`, verdict.Ask},
		{"abbreviated --no-dry cancels it", "prot", `git push -n --no-dry origin main`, verdict.Ask},
		{"--dry-run after --no-dry-run", "prot", `git push --no-dry-run --dry-run`, verdict.Silent},

		{"here-string to bash -s", "free", `bash -s <<< 'git -C PROT push'`, verdict.Ask},
		{"echo piped into sh", "free", `echo 'git -C PROT push' | sh`, verdict.Ask},
		{"echo piped into sudo bash", "free", `echo "cd PROT && git push" | sudo bash`, verdict.Ask},
		{"printf piped into bash", "free", `printf 'git -C %s push\n' PROT | bash`, verdict.Ask},
		{"echo piped into sh, then a filter", "free", `echo 'git -C PROT push' | sh | cat`, verdict.Ask},
		{"echo piped into a filter", "free", `echo 'git -C PROT push' | grep push`, verdict.Silent},
		{"here-string to a filter", "free", `grep push <<< 'git -C PROT push'`, verdict.Silent},
		{"echo piped into ssh", "free", `echo 'git push' | ssh host`, verdict.Ask},

		{"ssh.github.com on port 443", "free", `git push ssh://git@ssh.github.com:443/ElXreno/nixos-config.git main`, verdict.Ask},
		{"www.github.com", "free", `git push https://www.github.com/ElXreno/nixos-config main`, verdict.Ask},
		{"insteadOf from GIT_CONFIG_COUNT", "free", `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=url.` + protectedURL + `.insteadOf GIT_CONFIG_VALUE_0=https://example.com/x git push https://example.com/x main`, verdict.Ask},
		{"insteadOf from exported GIT_CONFIG_COUNT", "free", `export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=remote.origin.pushurl GIT_CONFIG_VALUE_0=` + protectedURL + `; git push`, verdict.Ask},
		{"unknown credential helper", "free", `git -c "credential.https://github.com.helper=$HELPER" push -u origin main`, verdict.Silent},
		{"unknown pushurl", "free", `git -c "remote.origin.pushurl=$AG_PUSH_TARGET_UNSET" push`, verdict.Ask},
		{"unknown config key", "free", `git -c "$AG_PUSH_KEY_UNSET=x" push`, verdict.Ask},
		{"unknown GIT_CONFIG helper value", "free", `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.helper GIT_CONFIG_VALUE_0=$HELPER git push`, verdict.Silent},
		{"unknown GIT_CONFIG value", "free", `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=remote.origin.pushurl GIT_CONFIG_VALUE_0=$AG_PUSH_TARGET_UNSET git push`, verdict.Ask},

		{"clone then push in the same line", "free", `git clone ` + protectedURL + ` ROOT/fresh && cd ROOT/fresh && git push`, verdict.Ask},
		{"clone of an unprotected repo then push", "free", `git clone https://github.com/ElXreno/other.git ROOT/fresh2 && cd ROOT/fresh2 && git push`, verdict.Silent},

		{"gh repo sync", "prot", `gh repo sync`, verdict.Silent},
		{"gh repo sync of the protected repo", "free", `gh repo sync ElXreno/nixos-config --source ElXreno/nixos-config`, verdict.Silent},
		{"push in an echo", "prot", `echo "git push"`, verdict.Silent},
		{"push in a log grep", "prot", `git log --grep push`, verdict.Silent},
		{"push.default config", "prot", `git config push.default simple`, verdict.Silent},
		{"set-url --push", "prot", `git remote set-url --push origin x`, verdict.Silent},
	}
	for _, tc := range cases {
		v := f.push(tc.cmd, tc.cwd)
		got := verdict.Silent
		if v != nil {
			got = v.Level
		}
		if got != tc.want {
			msg := ""
			if v != nil {
				msg = v.Message
			}
			t.Errorf("%s: want %v got %v for %q in %q\n  %s", tc.name, tc.want, got, tc.cmd, tc.cwd, msg)
		}
	}
}

func TestGitPushMessages(t *testing.T) {
	f := newPushFixture(t)
	cases := []struct {
		cwd, cmd string
		want     []string
		not      []string
	}{
		{"prot", `git push`, []string{"git push to github.com/ElXreno/nixos-config, a protected repository", "Yes only if the user asked for this push"}, nil},
		{"free", `git push https://x-access-token:s3cr3tvalue@github.com/ElXreno/nixos-config.git`, []string{"github.com/ElXreno/nixos-config"}, []string{"s3cr3tvalue", "x-access-token"}},
		{"prot", `git push $AG_PUSH_TARGET_UNSET`, []string{"from a checkout of github.com/ElXreno/nixos-config", "$AG_PUSH_TARGET_UNSET is only known when the command runs"}, nil},
		{"prot", `git push nosuchremote`, []string{"it names nosuchremote, which is no remote of this checkout"}, nil},
		{"free", `git -C $AG_PUSH_DIR_UNSET push`, []string{"git push, and the directory it runs in is only known when the command runs, so it may publish to a protected repository"}, nil},
		{"free", `ssh host git push`, []string{"on another host or in a container"}, nil},
	}
	for _, c := range cases {
		v := f.push(c.cmd, c.cwd)
		if v == nil {
			t.Errorf("%q: want an ask", c.cmd)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(v.Message, w) {
				t.Errorf("%q: message lacks %q: %s", c.cmd, w, v.Message)
			}
		}
		for _, n := range c.not {
			if strings.Contains(v.Message, n) {
				t.Errorf("%q: message leaks %q: %s", c.cmd, n, v.Message)
			}
		}
	}
}

func TestGitPushFailsClosedWithoutGit(t *testing.T) {
	f := newPushFixture(t)
	saved := gitProgram
	gitProgram = filepath.Join(f.root, "no-such-git")
	defer func() { gitProgram = saved }()
	v := f.push(`git push`, "free")
	if v == nil || v.Level != verdict.Ask || !strings.Contains(v.Message, "could not read the remotes") {
		t.Errorf("an unreadable checkout asks, got %+v", v)
	}
	f.cfg.ProtectedRepos = nil
	if v := f.push(`git push`, "prot"); v != nil {
		t.Errorf("no protected repositories, no check, no git run: %+v", v)
	}
}

func TestGitPushSSHAlias(t *testing.T) {
	f := newPushFixture(t)
	if _, err := exec.LookPath(sshProgram); err != nil {
		t.Skipf("no %s: %v", sshProgram, err)
	}
	conf := filepath.Join(f.root, "ssh_config")
	if err := os.WriteFile(conf, []byte("Host gh\n  HostName github.com\nHost elsewhere\n  HostName example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := sshArgs
	sshArgs = []string{"-F", conf}
	defer func() { sshArgs = saved }()
	repo(t, filepath.Join(f.root, "aliased"), true, "remote.origin.url", "gh:ElXreno/nixos-config")
	cases := []struct {
		cwd, cmd string
		want     verdict.Level
	}{
		{"free", `git push git@gh:ElXreno/nixos-config.git main`, verdict.Ask},
		{"free", `git push ssh://git@gh/ElXreno/nixos-config main`, verdict.Ask},
		{"aliased", `git push`, verdict.Ask},
		{"free", `git push git@elsewhere:ElXreno/nixos-config.git main`, verdict.Silent},
		{"free", `git push https://gh/ElXreno/nixos-config main`, verdict.Silent},
		{"free", `git push -- -oProxyCommand=x:ElXreno/nixos-config main`, verdict.Silent},
	}
	for _, c := range cases {
		v := f.push(c.cmd, c.cwd)
		got := verdict.Silent
		if v != nil {
			got = v.Level
		}
		if got != c.want {
			t.Errorf("%q in %q: want %v got %v", c.cmd, c.cwd, c.want, got)
		}
	}
}

func TestRepoKey(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://github.com/ElXreno/nixos-config", "github.com/ElXreno/nixos-config"},
		{"https://github.com/ElXreno/nixos-config.git/", "github.com/ElXreno/nixos-config"},
		{"https://user:token@github.com/ElXreno/nixos-config.git", "github.com/ElXreno/nixos-config"},
		{"ssh://git@github.com:22/ElXreno/nixos-config.git", "github.com/ElXreno/nixos-config"},
		{"git+ssh://git@github.com/ElXreno/nixos-config", "github.com/ElXreno/nixos-config"},
		{"git@github.com:ElXreno/nixos-config.git", "github.com/ElXreno/nixos-config"},
		{"github.com:/ElXreno/nixos-config", "github.com/ElXreno/nixos-config"},
		{"ssh://[::1]:2222/srv/repo.git", "[::1]/srv/repo"},
		{"file:///srv/git/x.git", "/srv/git/x"},
		{"/srv/git/x.git", "/srv/git/x.git"},
		{"../free", "../free"},
	}
	for _, c := range cases {
		if got := repoKey(c.url); got != c.want {
			t.Errorf("repoKey(%q): want %q got %q", c.url, c.want, got)
		}
	}
}

func TestProtectedPatterns(t *testing.T) {
	cfg := &Settings{ProtectedRepos: []string{protectedURL, "gitlab.com/team/*"}}
	for url, want := range map[string]bool{
		"https://github.com/ElXreno/nixos-config":               true,
		"https://gitlab.com/team/infra.git":                     true,
		"https://gitlab.com/team/infra/sub":                     false,
		"https://github.com/ElXreno/nixos-config-fork":          false,
		"ssh://git@ssh.github.com:443/ElXreno/nixos-config.git": true,
		"https://a.b.github.com/elxreno/nixos-config":           true,
		"https://evilgithub.com/ElXreno/nixos-config":           false,
		"https://github.com.evil.example/ElXreno/nixos-config":  false,
		"https://sub.gitlab.com/team/infra":                     true,
	} {
		if _, got := cfg.protectedKey(url); got != want {
			t.Errorf("%s: want %v got %v", url, want, got)
		}
	}
	for p, bad := range map[string]bool{"github.com/ElXreno/*": false, "github.com/[x": true, " ": true} {
		if err := ValidRepoPattern(p); (err != nil) != bad {
			t.Errorf("ValidRepoPattern(%q): %v", p, err)
		}
	}
}

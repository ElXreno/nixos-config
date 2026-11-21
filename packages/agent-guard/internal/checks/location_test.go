package checks

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"agent-guard/internal/harness"
	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

func locationFixture(t *testing.T) (*Settings, Context, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	repo := filepath.Join(root, "repo")
	for _, d := range []string{home, work, repo, filepath.Join(repo, ".git"), filepath.Join(root, "scratch"), filepath.Join(home, "projects", "repos"), filepath.Join(work, "dir")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(work, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &Settings{
		Home:        home,
		CreateRoots: []string{filepath.Join(home, ".claude")},
		ScratchBase: filepath.Join(root, "scratch"),
		ReposRoot:   filepath.Join(home, "projects", "repos"),
		AvoidRoots:  []string{home, "/tmp", "/mnt/datalake"},
	}
	ctx := Context{Cwd: work, SessionID: "77c13fc0", Today: "2026-09-05"}
	return cfg, ctx, root
}

func TestLocation(t *testing.T) {
	cfg, ctx, root := locationFixture(t)
	home := cfg.Home
	repo := filepath.Join(root, "repo")
	cases := []struct {
		name string
		cwd  string
		cmd  string
		want verdict.Level
	}{
		{"new file in work dir", "", `echo x > new.txt`, verdict.Advise},
		{"append to existing file", "", `echo x >> existing.txt`, verdict.Silent},
		{"dev null", "", `cmd > /dev/null 2>&1`, verdict.Silent},
		{"stderr dup", "", `cmd 2>&1`, verdict.Silent},
		{"variable target", "", `echo x > $OUT/file`, verdict.Silent},
		{"mkdir in home", "", `mkdir -p ` + home + `/tmp/probe`, verdict.Advise},
		{"mkdir in scratch", "", `mkdir -p ` + filepath.Join(root, "scratch", "p", "2026-09-05-x-77c1"), verdict.Silent},
		{"cp into existing dir", "", `cp a.txt dir/`, verdict.Advise},
		{"cp into scratch", "", `cp a.txt ` + filepath.Join(root, "scratch") + `/`, verdict.Silent},
		{"cd then write in scratch", "", `cd ` + filepath.Join(root, "scratch") + ` && echo x > out.log`, verdict.Silent},
		{"cd home then write", "", `cd ~ && echo x > notes.md`, verdict.Advise},
		{"cd inside a payload applies to the payload", "", `bash -c 'cd ` + filepath.Join(root, "scratch") + ` && echo x > out.log'`, verdict.Silent},
		{"cd inside a payload stays inside it", "", `bash -c 'cd ` + filepath.Join(root, "scratch") + `'; echo x > out.log`, verdict.Advise},
		{"pushd then write in scratch", "", `pushd ` + filepath.Join(root, "scratch") + ` && echo x > out.log`, verdict.Silent},
		{"popd returns to the work dir", "", `pushd ` + filepath.Join(root, "scratch") + `; popd; echo x > out.log`, verdict.Advise},
		{"tee", "", `make 2>&1 | tee build.log`, verdict.Advise},
		{"curl -o", "", `curl -sL -o page.html https://example.org/x`, verdict.Advise},
		{"curl -O", "", `curl -sLO https://example.org/dump.tar.gz`, verdict.Advise},
		{"wget default name", "", `wget https://example.org/a/b.zip`, verdict.Advise},
		{"git clone in clones root", filepath.Join(home, "projects", "repos"), `git clone https://github.com/o/r.git`, verdict.Silent},
		{"git clone in home", home, `git clone --depth 1 https://github.com/o/r.git`, verdict.Advise},
		{"nix build result link", "", `nix build .#x`, verdict.Advise},
		{"nix build no-link", "", `nix build --no-link .#x`, verdict.Silent},
		{"nix build inside repo", repo, `nix build .#x`, verdict.Silent},
		{"mktemp in /tmp", "", `d=$(mktemp -d)`, verdict.Advise},
		{"mktemp in scratch", "", `d=$(mktemp -d -p ` + filepath.Join(root, "scratch") + `)`, verdict.Silent},
		{"python literal target", "", `python3 -c "open('` + home + `/out.json','w').write('{}')"`, verdict.Advise},
		{"python target in repo", repo, `python3 -c "open('gen.json','w').write('{}')"`, verdict.Silent},
		{"remote payload ignored", "", `ssh host "echo x > /root/new.txt"`, verdict.Silent},
		{"write inside repo", repo, `git diff > patches/x.patch`, verdict.Silent},
		{"heredoc to new file", "", "cat > tool.py <<'PY'\nprint(1)\nPY", verdict.Advise},
		{"dd of", "", `dd if=/dev/zero of=disk.img bs=1M count=1`, verdict.Advise},
		{"harness task dir", "", `echo x > ` + filepath.Join(os.TempDir(), "claude-"+strconv.Itoa(os.Getuid()), "x", "out"), verdict.Silent},
		{"variable target in home", "", `T=` + home + `/probe; mkdir -p $T/sys && echo x > $T/out`, verdict.Advise},
		{"variable target in scratch", "", `D=` + filepath.Join(root, "scratch", "s") + `; mkdir -p $D; echo x > $D/out`, verdict.Silent},
		{"variable from command substitution stays opaque", "", `D=$(cat /tmp/.where); echo x > $D/out`, verdict.Silent},
		{"prefix assignment does not reach the same command", "", `M=` + home + `/lib cp a.txt "$M"/`, verdict.Silent},
		{"exported variable", "", `export OUT=` + home + `/out.txt; echo x > $OUT`, verdict.Advise},
	}
	for _, tc := range cases {
		c := ctx
		if tc.cwd != "" {
			c.Cwd = tc.cwd
		}
		v := location(shell.Analyze(tc.cmd), c, cfg)
		got := verdict.Silent
		if v != nil {
			got = v.Level
		}
		if got != tc.want {
			msg := ""
			if v != nil {
				msg = v.Message
			}
			t.Errorf("%s: want %v got %v for %q\n  %s", tc.name, tc.want, got, tc.cmd, msg)
		}
	}
}

func TestLocationMessage(t *testing.T) {
	cfg, ctx, root := locationFixture(t)
	v := location(shell.Analyze(`echo x > new.txt`), ctx, cfg)
	if v == nil {
		t.Fatal("expected an advisory")
	}
	want := filepath.Join(root, "scratch", "work", "2026-09-05-<topic>-77c1")
	if !strings.Contains(v.Message, want) {
		t.Errorf("message should name %s, got: %s", want, v.Message)
	}
	if !strings.HasSuffix(v.Message, "/ (mkdir -p it first, pick a short topic); repository files and clones under "+filepath.Join(cfg.Home, "projects", "repos")+" are fine where they are. Not $HOME, not /tmp, not /mnt/datalake.") {
		t.Errorf("the configured roots render as before: %s", v.Message)
	}
	bare := &Settings{Home: cfg.Home}
	if m := locationMsg([]string{"/x"}, ctx, bare, "", ctx.Cwd); m != "New files outside the agreed scratch area: /x." {
		t.Errorf("nothing configured, nothing promised: %q", m)
	}
}

func TestLocationForWrite(t *testing.T) {
	cfg, ctx, root := locationFixture(t)
	if v := LocationForNewFile(filepath.Join(cfg.Home, "analysis.md"), ctx, cfg); v == nil {
		t.Error("new file in home should be advised")
	}
	if v := LocationForNewFile(filepath.Join(ctx.Cwd, "existing.txt"), ctx, cfg); v != nil {
		t.Error("existing file should be silent")
	}
	if v := LocationForNewFile(filepath.Join(root, "repo", "src", "new.go"), ctx, cfg); v != nil {
		t.Error("new file inside a repository should be silent")
	}
	if v := LocationForNewFile(filepath.Join(cfg.Home, ".claude", "projects", "x", "memory", "note.md"), ctx, cfg); v != nil {
		t.Error("memory file should be silent")
	}
}

func TestLocationNamesTheRealTarget(t *testing.T) {
	cfg, ctx, root := locationFixture(t)
	repo := filepath.Join(root, "repo")
	cases := []struct {
		name, cmd, want string
	}{
		{"cd to a bare tilde", `cd ~ && echo x > notes.md`, filepath.Join(cfg.Home, "notes.md")},
		{"out-link with an equals sign", `nix build --out-link=` + cfg.Home + `/res .#x`, filepath.Join(cfg.Home, "res")},
		{"nix-build out-link with an equals sign", `nix-build --out-link=` + cfg.Home + `/res2 x.nix`, filepath.Join(cfg.Home, "res2")},
		{"nix build -o", `nix build -o ` + cfg.Home + `/res3 .#x`, filepath.Join(cfg.Home, "res3")},
	}
	for _, c := range cases {
		v := location(shell.Analyze(c.cmd), ctx, cfg)
		if v == nil || !strings.Contains(v.Message, c.want+".") && !strings.Contains(v.Message, c.want+",") {
			t.Errorf("%s: want the advice to name %s, got %v", c.name, c.want, v)
		}
	}
	for _, cmd := range []string{
		"cd " + repo + " && cat > modules/new.nix <<EOF\n{ }\nEOF",
		"echo x > " + filepath.Join(repo, "notes.md"),
	} {
		if v := location(shell.Analyze(cmd), ctx, cfg); v != nil {
			t.Errorf("%q: a new file inside another repository is fine, got %s", cmd, v.Message)
		}
	}
	if (location(shell.Analyze("echo x > "+filepath.Join(repo, "a.md")), ctx, cfg) == nil) != (LocationForNewFile(filepath.Join(repo, "a.md"), ctx, cfg) == nil) {
		t.Error("a Write and a redirect into another repository must agree")
	}
}

func TestGitAddClusters(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{"git add -uv", "deny"},
		{"git add -vA", "deny"},
		{"git add -v file", "silent"},
	} {
		if got := runOne(GitScope, shell.Analyze(c.cmd), testSettings).Worst().String(); got != c.want {
			t.Errorf("%s: want %s got %s", c.cmd, c.want, got)
		}
	}
}

func TestTaskDirFollowsTheHarness(t *testing.T) {
	cc, _ := harness.ByID(string(harness.ClaudeCode))
	pi, _ := harness.ByID(string(harness.Pi))
	taskDir := cc.TaskDir()
	if taskDir == "" {
		t.Fatal("claude-code must have a task dir")
	}
	if !slices.Contains((&Settings{Spec: cc}).createRoots(""), taskDir) {
		t.Errorf("claude-code create roots must include %s", taskDir)
	}
	if slices.Contains((&Settings{Spec: pi}).createRoots(""), taskDir) {
		t.Errorf("pi create roots must not include %s", taskDir)
	}
}

func TestReadOnlyRoots(t *testing.T) {
	cfg := &Settings{Home: "/home/test", ReadOnlyRoots: []ReadOnlyRoot{{Path: "~/.claude", Reason: "it belongs to Claude Code"}}}
	ctx := Context{Cwd: "/home/test/repo"}
	cases := []struct {
		cmd  string
		deny bool
	}{
		{"echo x > /home/test/.claude/x", true},
		{"cd ~/.claude && echo x > notes.md", true},
		{"cp a ~/.claude/projects/p/memory/x.md", true},
		{"cat /home/test/.claude/x", false},
		{"echo x > /home/test/.claudex", false},
		{"ssh host 'echo x > /home/test/.claude/x'", false},
	}
	for _, c := range cases {
		v := runOneWith(ReadOnly, shell.Analyze(c.cmd), ctx, cfg)
		if got := len(v) > 0 && v[0].Level == verdict.Deny; got != c.deny {
			t.Errorf("%q: want deny %v, got %v", c.cmd, c.deny, v)
		}
		if c.deny && len(v) > 0 && !strings.Contains(v[0].Message, "/home/test/.claude, which is read-only here: it belongs to Claude Code") {
			t.Errorf("%q: message %q", c.cmd, v[0].Message)
		}
	}
	if v := runOneWith(ReadOnly, shell.Analyze("echo x > /home/test/.claude/x"), ctx, &Settings{Home: "/home/test"}); len(v) != 0 {
		t.Errorf("no read-only roots, no verdict: %v", v)
	}
	if v := ReadOnlyForFile("/home/test/.claude/settings.json", ctx, cfg); v == nil || v.Check != ReadOnly {
		t.Errorf("an existing file under a read-only root is blocked too: %v", v)
	}
	if v := ReadOnlyForFile("../x.md", ctx, cfg); v != nil {
		t.Errorf("a path outside the root is fine: %v", v)
	}
}

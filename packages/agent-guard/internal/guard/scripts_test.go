package guard

import (
	"os"
	"strings"
	"testing"

	"agent-guard/internal/call"
	"agent-guard/internal/checks"
	"agent-guard/internal/edit"
	"agent-guard/internal/treesitter"
	"agent-guard/internal/treesitter/treesittertest"
	"agent-guard/internal/verdict"
)

func engine(files map[string]string) *Engine {
	read := func(p string) (string, bool) {
		s, ok := files[p]
		return s, ok
	}
	return &Engine{
		Checks:     &checks.Settings{Home: "/home/u", ScratchBase: "/mnt/scratch", ReposRoot: "/home/u/repos"},
		Edit:       edit.Options{Read: read, Previous: read},
		ReadScript: read,
	}
}

func shellCall(cmd string) call.Call {
	return call.Call{Kind: call.Shell, Command: cmd, Cwd: "/home/u/repos/x", Session: "s"}
}

func TestScriptsRun(t *testing.T) {
	e := engine(map[string]string{
		"/home/u/repos/x/deploy.sh": "#!/usr/bin/env bash\nset -e\nnixos-rebuild switch --flake .\n",
		"/home/u/repos/x/paths.sh":  "#!/bin/sh\nobjdump=$(nix build --print-out-paths nixpkgs#binutils)/bin/objdump\n",
		"/home/u/repos/x/outer.sh":  "bash ./paths.sh\n",
		"/home/u/repos/x/loop.sh":   "source ./loop.sh\n",
		"/home/u/repos/x/tool":      "nixos-rebuild switch\n",
		"/home/u/repos/x/ok.sh":     "#!/bin/sh\necho fine\n",
		"/home/u/repos/x/stand.sh":  "#!/usr/bin/env bash\nserver --port 8150 &\nsleep 60\ngrep -c ready log.txt\nkill %1\n",
		"/home/u/repos/x/mixed.sh":  "#!/usr/bin/env bash\nserver &\nsleep 60\nnixos-rebuild switch\n",
	})
	cases := []struct {
		cmd, check string
		want       verdict.Level
	}{
		{`./deploy.sh`, checks.Deploy, verdict.Deny},
		{`cd /tmp && cd /home/u/repos/x && ./deploy.sh`, checks.Deploy, verdict.Deny},
		{`cd sub && bash ../paths.sh`, checks.NixOutputs, verdict.Deny},
		{`cd /tmp && ./deploy.sh`, "", verdict.Silent},
		{`cd "$X" && ./deploy.sh`, "", verdict.Silent},
		{`bash deploy.sh`, checks.Deploy, verdict.Deny},
		{`cd /tmp; bash /home/u/repos/x/paths.sh`, checks.NixOutputs, verdict.Deny},
		{`bash outer.sh`, checks.NixOutputs, verdict.Deny},
		{`source loop.sh`, "", verdict.Silent},
		{`./tool`, "", verdict.Silent},
		{`./ok.sh`, "", verdict.Silent},
		{`bash missing.sh`, "", verdict.Silent},
		{`bash "$DIR/deploy.sh"`, "", verdict.Silent},
		{`./stand.sh`, "", verdict.Silent},
		{`bash -n deploy.sh`, "", verdict.Silent},
		{`./mixed.sh`, checks.Deploy, verdict.Deny},
	}
	for _, c := range cases {
		d := e.PreTool(shellCall(c.cmd))
		got, check, msg := verdict.Silent, "", ""
		for _, v := range d.Verdicts {
			if v.Level > got {
				got, check, msg = v.Level, v.Check, v.Message
			}
		}
		if got != c.want || check != c.check {
			t.Errorf("%q: got %s %s, want %s %s", c.cmd, got, check, c.want, c.check)
		}
		if c.want != verdict.Silent && !strings.HasPrefix(msg, "In script /home/u/repos/x/") {
			t.Errorf("%q: the message names the script: %q", c.cmd, msg)
		}
	}
}

func TestScriptsOffInReplay(t *testing.T) {
	e := engine(map[string]string{"/home/u/repos/x/deploy.sh": "nixos-rebuild switch\n"})
	e.ReadScript = nil
	if d := e.PreTool(shellCall(`bash deploy.sh`)); len(d.Verdicts.At(verdict.Deny)) != 0 {
		t.Errorf("without ReadScript no file is read: %+v", d.Verdicts)
	}
}

func adviceFor(t *testing.T, e *Engine, c call.Change) verdict.Verdicts {
	t.Helper()
	kind := call.Edit
	if c.Write {
		kind = call.Write
	}
	var out verdict.Verdicts
	for _, v := range e.PreTool(call.Call{Kind: kind, Change: c, Cwd: "/home/u/repos/x"}).Verdicts {
		if v.Check == checks.NixOutputs || v.Check == checks.RgReplace || v.Check == checks.Pkill || v.Check == checks.NixStoreRoot {
			out = append(out, v)
		}
	}
	return out
}

func TestScriptAdviceOnWrite(t *testing.T) {
	bad := "objdump=$(nix build --print-out-paths nixpkgs#binutils)/bin/objdump\n"
	e := engine(map[string]string{"/home/u/repos/x/old.sh": "#!/bin/sh\n" + bad})

	got := adviceFor(t, e, call.Change{Path: "/home/u/repos/x/new.sh", Write: true, Content: bad + "rg -rn foo .\n"})
	if len(got) != 2 {
		t.Fatalf("a new script gets one note per finding, got %+v", got)
	}
	for _, v := range got {
		if v.Level != verdict.Advise || !strings.Contains(v.Message, "/home/u/repos/x/new.sh") {
			t.Errorf("script findings are advice naming the file: %+v", v)
		}
	}

	got = adviceFor(t, e, call.Change{Path: "/home/u/repos/x/old.sh", Pairs: []call.Pair{{Old: "#!/bin/sh\n", New: "#!/bin/sh\nset -e\n"}}})
	if len(got) != 0 {
		t.Errorf("a finding that was already in the file is not repeated: %+v", got)
	}

	got = adviceFor(t, e, call.Change{Path: "/home/u/repos/x/notes.md", Write: true, Content: bad})
	if len(got) != 0 {
		t.Errorf("a non-script file is not analysed: %+v", got)
	}
}

func TestScriptAdviceInNix(t *testing.T) {
	path := treesittertest.Path(t)
	g, err := treesitter.FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	e := engine(nil)
	e.Edit.Grammars = g
	nix := `{ pkgs, ... }: {
  a = pkgs.writeShellApplication {
    name = "a";
    text = ''
      p=$(nix build --print-out-paths ${pkgs.hello}^out nixpkgs#binutils)
    '';
  };
  b = pkgs.writeShellScript "b" ''
    pkill -f foo
  '';
  c.postInstall = ''
    rg -rn x .
  '';
  environment.etc."x.conf".text = ''
    rg -rn not a script
  '';
}
`
	got := adviceFor(t, e, call.Change{Path: "/home/u/repos/x/pkg.nix", Write: true, Content: nix})
	checksSeen := map[string]bool{}
	for _, v := range got {
		checksSeen[v.Check] = true
	}
	if len(got) != 3 || !checksSeen[checks.NixOutputs] || !checksSeen[checks.Pkill] || !checksSeen[checks.RgReplace] {
		t.Errorf("the three shell strings are analysed, the config text is not: %+v", got)
	}
}

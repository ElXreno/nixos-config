package shell

import "testing"

func TestScriptFile(t *testing.T) {
	cases := []struct {
		cmd, path string
		direct    bool
	}{
		{`bash run.sh arg`, "run.sh", false},
		{`sh -e ./x.sh`, "./x.sh", false},
		{`bash -o pipefail -- /tmp/a.sh`, "/tmp/a.sh", false},
		{`source ~/env.sh`, "~/env.sh", false},
		{`. ./lib.sh`, "./lib.sh", false},
		{`./deploy.sh --now`, "./deploy.sh", true},
		{`timeout 60 /opt/x/run`, "/opt/x/run", true},
		{`bash -c 'echo hi'`, "", false},
		{`bash -s < x.sh`, "", false},
		{`bash`, "", false},
		{`ls ./dir`, "", false},
	}
	for _, c := range cases {
		cmds := Analyze(c.cmd).Commands
		path, direct, ok := "", false, false
		for _, cmd := range cmds {
			if p, d, o := ScriptFile(cmd); o {
				path, direct, ok = p, d, o
				break
			}
		}
		if path != c.path || direct != c.direct || ok != (c.path != "") {
			t.Errorf("%q: got %q direct=%v ok=%v, want %q direct=%v", c.cmd, path, direct, ok, c.path, c.direct)
		}
	}
	for src, want := range map[string]bool{
		"#!/usr/bin/env bash\nset -e": true,
		"#!/bin/sh\n":                 true,
		"#!/usr/bin/env -S bash -eu":  true,
		"#!/usr/bin/env python3\n":    false,
		"echo no shebang":             false,
	} {
		if ShellShebang(src) != want {
			t.Errorf("ShellShebang(%q) = %v", src, !want)
		}
	}
}

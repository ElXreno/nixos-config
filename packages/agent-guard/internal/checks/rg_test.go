package checks

import (
	"testing"

	"agent-guard/internal/shell"
)

func TestRgFix(t *testing.T) {
	cases := []struct {
		cmd, want string
	}{
		{`rg -rn foo src/`, `rg -n foo src/`},
		{`rg -nr foo .`, `rg -n foo .`},
		{`rg -rli pattern`, `rg -li pattern`},
		{`cd x && rg -rn foo | head`, `cd x && rg -n foo | head`},
		{`rg -r foo .`, ""},
		{`ssh host "rg -rn foo /etc"`, ""},
		{`bash -c "rg -rn foo"`, ""},
		{`rg -rn -- "-rn" .`, `rg -n -- "-rn" .`},
		{`rg -rn -- -rn .`, ""},
		{`rg -rn a; rg -rn b`, ""},
		{`rg -rn -ri foo`, ""},
		{`rg -rn foo . && rg -rl bar .`, ""},
	}
	for _, c := range cases {
		v := runOne(RgReplace, shell.Analyze(c.cmd), testSettings)
		if len(v) == 0 {
			t.Errorf("%q: want an rg-replace deny", c.cmd)
			continue
		}
		fix := ""
		for _, x := range v {
			if x.Fix != "" {
				fix = x.Fix
			}
		}
		if fix != c.want {
			t.Errorf("%q: want fix %q got %q", c.cmd, c.want, fix)
		}
	}
	if v := runOne(RgReplace, shell.Analyze(`rg -n foo .`), testSettings); len(v) != 0 {
		t.Errorf("rg -n needs no fix, got %v", v)
	}
}

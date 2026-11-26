package checks

import (
	"testing"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

func TestNixOutputs(t *testing.T) {
	cases := []struct {
		want verdict.Level
		cmd  string
	}{
		{verdict.Deny, `nix build --no-link --print-out-paths nixpkgs#binutils | head -1`},
		{verdict.Deny, `p=$(nix build --no-link --print-out-paths nixpkgs#binutils)/bin/objdump`},
		{verdict.Deny, `ls "$(nix build --print-out-paths .#foo)"/bin`},
		{verdict.Deny, `nix build --print-out-paths nixpkgs#hello^out nixpkgs#binutils | xargs ls`},
		{verdict.Deny, `nix build -L --print-out-paths | tail -1`},
		{verdict.Deny, `diff <(nix build --print-out-paths .#a) old.txt`},
		{verdict.Deny, `x=$(nix build --print-out-paths .#nixosConfigurations.SMOK.config.system.build.toplevel)`},
		{verdict.Deny, `bash -c 'nix build --print-out-paths nixpkgs#zlib | head -1'`},
		{verdict.Deny, `nix build --print-out-paths .#a | while read p; do ls "$p"; done`},
		{verdict.Deny, `nix build --print-out-paths .#a 2>/dev/null | sed -n 1p | xargs ls`},
		{verdict.Silent, `nix build --print-out-paths .#a | tee paths.txt`},
		{verdict.Silent, `nix build --no-link --print-out-paths nixpkgs#binutils`},
		{verdict.Silent, `nix build --print-out-paths nixpkgs#binutils 2>&1 | rg -v warning`},
		{verdict.Silent, `nix build --no-link --print-out-paths nixpkgs#binutils^bin | head -1`},
		{verdict.Silent, `p=$(nix build --print-out-paths 'nixpkgs#zlib^dev,out')`},
		{verdict.Silent, `p=$(nix build --print-out-paths nixpkgs#legacyPackages.aarch64-linux.hello.out)`},
		{verdict.Silent, `p=$(nix build --print-out-paths .#pkgs.elxreno-clan.rea-full.bin)`},
		{verdict.Silent, `p=$(nix build --print-out-paths /nix/store/abc-hello-2.12.3)`},
		{verdict.Silent, `p=$(nix build --print-out-paths -o res --option cores 4 nixpkgs#hello^out)`},
		{verdict.Silent, `p=$(nix build --no-link nixpkgs#hello)`},
		{verdict.Silent, `p=$(nix eval --raw nixpkgs#hello.outPath)`},
		{verdict.Silent, `echo "$(nix path-info nixpkgs#hello)"`},
	}
	for _, c := range cases {
		got := verdict.Silent
		for _, v := range runOne(NixOutputs, shell.Analyze(c.cmd), testSettings) {
			got = v.Level
		}
		if got != c.want {
			t.Errorf("%q: want %s, got %s", c.cmd, c.want, got)
		}
	}
}

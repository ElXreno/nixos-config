package checks

import (
	"testing"

	"agent-guard/internal/shell"
)

var deployMatrix = []struct {
	want string
	cmd  string
}{
	{"deny", `nixos-rebuild switch --flake .#KURWA`},
	{"deny", `sudo nixos-rebuild switch --flake .`},
	{"deny", `nixos-rebuild --flake .#BIMBA --target-host root@bimba switch`},
	{"deny", `nixos-rebuild boot --flake .`},
	{"deny", `nixos-rebuild test --flake .`},
	{"deny", `nixos-rebuild-ng switch --flake .`},
	{"deny", `ssh root@bimba "nixos-rebuild switch --flake github:ElXreno/nixos-config#BIMBA"`},
	{"deny", `ssh root@grate bash -c "cd /etc/nixos && nixos-rebuild switch"`},
	{"deny", `home-manager switch --flake .#elxreno@KURWA`},
	{"deny", `/nix/var/nix/profiles/system/bin/switch-to-configuration switch`},
	{"deny", `sudo /run/current-system/bin/switch-to-configuration test`},
	{"deny", `sudo result/bin/switch-to-configuration boot`},
	{"deny", `clan machines update BIMBA`},
	{"deny", `clan machines install --target-host root@x GRATE`},
	{"deny", `nix run .#clan -- machines update KURWA`},
	{"deny", `nix run nixpkgs#nixos-rebuild -- switch --flake .`},
	{"deny", `nixos-anywhere --flake .#INFINITY root@infinity`},
	{"deny", `colmena apply --on bimba`},
	{"deny", `deploy .#bimba`},
	{"deny", `nh os switch .`},
	{"deny", `nh home switch`},
	{"deny", `cd ~/Sync/PC/share/nixos-config && nix build .#nixosConfigurations.KURWA.config.system.build.toplevel && sudo nixos-rebuild switch --flake .`},
	{"silent", `nix build .#nixosConfigurations.KURWA.config.system.build.toplevel --no-link --print-out-paths`},
	{"silent", `nixos-rebuild build --flake .#KURWA`},
	{"silent", `nixos-rebuild dry-build --flake .`},
	{"silent", `nixos-rebuild dry-activate --flake .`},
	{"silent", `nixos-rebuild build-vm --flake .`},
	{"silent", `nixos-rebuild list-generations`},
	{"silent", `home-manager build --flake .`},
	{"silent", `home-manager generations`},
	{"silent", `/run/current-system/bin/switch-to-configuration dry-activate`},
	{"silent", `clan machines list`},
	{"silent", `clan vars generate KURWA`},
	{"silent", `clan machines update --help`},
	{"silent", `colmena build`},
	{"silent", `nh search foo`},
	{"silent", `nh clean all`},
	{"silent", `systemctl restart nginx`},
	{"silent", `systemctl edit --runtime nginx`},
	{"silent", `ssh root@bimba "systemctl status nginx"`},
	{"silent", `git log --oneline -5 -- switch.sh`},
	{"silent", `echo "nixos-rebuild switch is for the user"`},
	{"silent", `rg -n 'nixos-rebuild switch' docs/`},
	{"silent", `./deploy.sh staging`},
	{"silent", `kubectl rollout restart deploy/api`},
}

func TestDeploy(t *testing.T) {
	for _, tc := range deployMatrix {
		a := shell.Analyze(tc.cmd)
		got := runOne("deploy", a, testSettings).Worst().String()
		if got != tc.want {
			t.Errorf("deploy %q: want %s got %s", tc.cmd, tc.want, got)
		}
	}
}

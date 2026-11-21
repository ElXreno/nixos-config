package checks

import (
	"regexp"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

const deployMsg = "Blocked: this activates a system or home-manager closure, and the user performs every deploy themselves. Build it (nix build .#nixosConfigurations.<host>.config.system.build.toplevel) and hand them the command to run. To experiment with a running service use what vanishes on reboot: systemctl edit --runtime, a drop-in under /run/systemd/system, a bind mount of one file over the store path, a unit restart."

var activatingSubcommands = map[string]bool{"switch": true, "boot": true, "test": true}

// Options of the deploy tools that consume the following word, so that
// `nixos-rebuild --flake .#HOST --target-host root@host switch` still yields
// `switch` as the subcommand.
var deployValueOpts = regexp.MustCompile(`^--(flake|file|attr|target-host|build-host|profile-name|specialisation|max-jobs|cores|option|override-input|update-input|log-format|builders|include|sudo-command|backup-extension|arg|argstr|config|on|hostname|configuration|specialization|output-dir|update)$`)

// deploy denies every command whose effect is to activate a built closure,
// whatever the wrapper: sudo, ssh, nix run and the rest are already peeled by
// the shell analysis. Builds, dry runs and --help pass.
func deploy(cmd shell.Command, _ *Settings) *verdict.Verdict {
	head := cmd.Head()
	args := cmd.Args()
	if contains(args, "--help", "-h") {
		return nil
	}
	rest := optPositionals(args, "fpjIcbo", deployValueOpts)
	first := ""
	if len(rest) > 0 {
		first = rest[0]
	}
	switch head {
	case "nixos-rebuild", "nixos-rebuild-ng", "switch-to-configuration":
		if activatingSubcommands[first] {
			return deny(deployMsg)
		}
	case "home-manager":
		if first == "switch" {
			return deny(deployMsg)
		}
	case "clan":
		if len(rest) >= 2 && rest[0] == "machines" && (rest[1] == "update" || rest[1] == "install" || rest[1] == "morph") {
			return deny(deployMsg)
		}
	case "nixos-anywhere", "nixos-install", "deploy":
		return deny(deployMsg)
	case "colmena":
		if first == "apply" || first == "apply-local" || first == "upload-keys" {
			return deny(deployMsg)
		}
	case "nh":
		if len(rest) >= 2 && (rest[0] == "os" || rest[0] == "home" || rest[0] == "darwin") && activatingSubcommands[rest[1]] {
			return deny(deployMsg)
		}
	}
	return nil
}

{
  config,
  namespace,
  lib,
  osConfig,
  ...
}:
let
  cfg = config.${namespace}.programs.claude-code;
  inherit (config.${namespace}.programs.agent-guard) scratchBase;
  tailnet = osConfig.clan.core.settings.domain;
in
{
  config = lib.mkIf cfg.enable {
    programs.claude-code.settings = {
      permissions.defaultMode = "auto";

      autoMode = {
        environment = [
          "$defaults"
          "Organization: one engineer's NixOS machines, single-user, no shared clusters or other people's workloads. Primary use: NixOS and home-manager configuration, Go and Rust tooling"
          "Source control: github.com/ElXreno. Repository visibility: repositories under github.com/ElXreno, cloned at ~/projects/repos/github.com/ElXreno, are public unless stated otherwise, nixos-config included; everything else is private unless stated otherwise"
          "Secrets management: clan vars and sops with age. Ciphertext under vars/per-machine/, vars/shared/, sops/ and *.age files is normal repository content; plaintext is never printed or committed"
          "Trusted internal domains: *.${tailnet}, the user's own single-user machines, none is production; ssh root@<host> with a one-shot command is the normal way to operate them"
          "Host containment: ordinary developer machines with open internet, no cloud metadata service, no container egress rules"
          "Additional context: /nix/store is read-only; /mnt/datalake is a sensitive data location holding the user's own data"
        ];
        allow = [
          "$defaults"
          "Agent scratch: ${scratchBase} is the agent's own space and each session works in its own subdirectory; creating, overwriting and deleting anything in the current session's directory is a Local Operation, other sessions' directories are pre-existing files"
          "Clones: repositories under ~/projects/repos are project scope for reading and for editing tracked files; uncommitted or untracked work in a pre-existing clone is not disposable, so git clean, reset --hard, checkout . and rm -rf of a clone still need the user to name the target; a clone whose origin remote is not under github.com/ElXreno is external code, running its scripts, builds or install steps stays Code from External"
          "Nix tooling: nix build, nix run, nix shell and nix-shell -p against nixpkgs or the inputs declared in this flake's flake.nix are routine tool invocation, not installs; a URL, --override-input or an arbitrary third-party flake is external code; nix run never clears Deploy Activation, judge the invoked program's effect, not the wrapper"
          "Own hosts over ssh: on the user's machines in *.${tailnet} this overrides Remote Shell Writes and Production Reads for one-shot commands: status queries, log reading, restarting a unit other than sshd, tailscaled or the network stack, and an experiment on a running service that vanishes on reboot; activating a closure there remains Deploy Activation"
          "Own hardware repair: the user repairs and reverse-engineers devices they own. Searching for, cloning and reading public repositories, forum threads, vendor service tools and firmware images for such a device, including sources that also describe offensive uses of the same firmware, is routine research on the user's own device, not Unauthorized Access or Third-Party Attack."
          "Client identity: sending a browser-like or otherwise custom User-Agent, Accept and Referer headers when reading publicly reachable pages and files (curl -A, wget -U, a header in a Go or Python script, agent-browser), including retrying that way after a 403 or a bot-filter page, and instructing a subagent to do so, is routine public reading, not Unauthorized Access, Third-Party Attack or Auto-Mode Bypass; a User-Agent filter is not an access control."
        ];
        soft_deny = [
          "$defaults"
          "Datalake [named+specifics - **must name:** the path under /mnt/datalake being written, deleted or copied out]: reading and analysing /mnt/datalake in place is routine; creating, changing or deleting anything there, or copying its contents into a repository, a commit or any outbound write, needs the user to name that path"
        ];
        hard_deny = [
          "$defaults"
          "Deploy Activation: the user performs every activation of a built closure themselves, and a request in the conversation does not change that. Never run nixos-rebuild switch, boot or test, nixos-rebuild-ng, nixos-rebuild --target-host, home-manager switch, nh os or nh home switch, switch-to-configuration by any path, clan machines update, install or morph, nixos-anywhere, colmena apply, deploy-rs, or any just, make, script or nix run target whose effect is to activate a system or home-manager closure, on any host, locally or over ssh. The permitted experiments are the ones that vanish on reboot: systemctl edit --runtime, a runtime drop-in under /run/systemd/system, bind-mounting one file over a store path, restarting the unit; anything that writes /etc, /nix/var/nix/profiles or otherwise persists across reboot is an activation. nix build and dry runs are not activations"
        ];
      };

      permissions.allow = [
        "Bash(chmod:*)"
        "Bash(curl:*)"
        "Bash(echo:*)"
        "Bash(fd:*)"
        "Bash(find:*)"
        "Bash(gh api:*)"
        "Bash(gh issue:*)"
        "Bash(gh pr:*)"
        "Bash(gh release:*)"
        "Bash(gh repo:*)"
        "Bash(gh run:*)"
        "Bash(gh search:*)"
        "Bash(git fetch:*)"
        "Bash(git rebase:*)"
        "Bash(git pull:*)"
        "Bash(git reflog:*)"
        "Bash(git cherry-pick:*)"
        "Bash(git cp:*)"
        "Bash(glab ci:*)"
        "Bash(grep:*)"
        "Bash(helm dependency build:*)"
        "Bash(helm dependency update:*)"
        "Bash(helm show:*)"
        "Bash(helm template:*)"
        "Bash(jq)"
        "Bash(kubectl describe:*)"
        "Bash(kubectl get:*)"
        "Bash(kubectl logs:*)"
        "Bash(kubectl version:*)"
        "Bash(kubectl wait:*)"
        "Bash(ls:*)"
        "Bash(agent-browser:*)"
        "Bash(nix:*)"
        "Bash(nix-shell:*)"
        "Bash(python3:*)"
        "Bash(rg:*)"
        "Bash(head:*)"
        "Bash(tail:*)"
        "Bash(test:*)"
        "Bash(wait)"
        "Bash(wc:*)"
        "Bash(xargs:*)"
        "Read(//home/*/.claude/**)"
        "Read(//tmp/**)"
        "WebSearch"
        "mcp__plugin_hm_ghidra-mcp__*"
      ];
    };
  };
}

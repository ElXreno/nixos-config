{
  imports = [
    ./machines.nix
    ./users.nix
    ./instances/crowdsec.nix
    ./instances/kopia.nix
    ./instances/modules.nix
    ./instances/monitoring.nix
    ./instances/pam-rssh.nix
    ./instances/roles.nix
    ./instances/sing-box.nix
    ./instances/sshd.nix
    ./instances/tailscale.nix
  ];
}

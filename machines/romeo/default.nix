{ inputs, ... }:

{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/scan/not-detected.nix"
    "${inputs.nixpkgs}/nixos/modules/profiles/qemu-guest.nix"
    inputs.disko.nixosModules.disko
    inputs.self.diskoConfigurations.hcloud
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.matrix-synapse
  ];

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  system.stateVersion = "23.05";
}

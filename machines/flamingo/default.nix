{ inputs, ... }:

{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/scan/not-detected.nix"
    "${inputs.nixpkgs}/nixos/modules/profiles/qemu-guest.nix"
    inputs.disko.nixosModules.disko
    inputs.self.diskoConfigurations.hcloud
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.hydra
    inputs.self.nixosProfiles.attic
    inputs.self.nixosProfiles.builder
  ];

  security.sudo.wheelNeedsPassword = false;

  system.stateVersion = "24.11";
}

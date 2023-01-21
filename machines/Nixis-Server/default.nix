{ config, inputs, pkgs, lib, ... }:

{
  imports =
    [
      ./wireguard.nix
      "${inputs.nixpkgs}/nixos/modules/virtualisation/azure-common.nix"
      inputs.self.nixosProfiles.nginx
      inputs.self.nixosRoles.server
    ];

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  system.stateVersion = "22.05";
}

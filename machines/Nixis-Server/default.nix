{ config, inputs, pkgs, lib, ... }:

{
  imports =
    [
      ./wireguard.nix
      "${inputs.nixpkgs}/nixos/modules/virtualisation/azure-common.nix"
      inputs.self.nixosRoles.server
    ];

  security.sudo.wheelNeedsPassword = false;

  system.stateVersion = "22.05";
}

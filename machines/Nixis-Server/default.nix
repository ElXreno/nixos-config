{ config, inputs, pkgs, lib, ... }:

{
  imports =
    [
      ./hardware-configuration.nix
      ./wireguard.nix
      inputs.self.nixosRoles.server
    ];

  boot.loader = {
    grub.device = "/dev/sda";
    timeout = 0;
    grub.configurationLimit = 0;
  };

  system.stateVersion = "22.05";
}

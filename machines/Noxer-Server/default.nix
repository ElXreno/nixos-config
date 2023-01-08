{ config, pkgs, inputs, lib, ... }:
{
  imports = [
    inputs.self.nixosRoles.server
    ./hardware-configuration.nix
    # ./wireguard.nix
  ];

  boot.loader.timeout = 0;
  boot.loader.systemd-boot.enable = true;
  boot.loader.systemd-boot.configurationLimit = 3;
  boot.loader.efi.canTouchEfiVariables = true;

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  # Move to hardware
  services.logind.lidSwitch = "ignore";

  system.stateVersion = "22.05";
}

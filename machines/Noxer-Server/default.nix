{ config, pkgs, inputs, lib, ... }:
{
  imports = [
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.minidlna
    ./hardware-configuration.nix
    ./qbittorrent-nox.nix
    # ./wireguard.nix
  ];

  boot.loader.timeout = 0;
  boot.loader.systemd-boot.enable = true;
  boot.loader.systemd-boot.configurationLimit = 3;
  boot.loader.efi.canTouchEfiVariables = true;

  boot.extraModprobeConfig = ''
    options ath9k btcoex_enable=0 bt_ant_diversity=0 ps_enable=0
    options cfg80211 ieee80211_regdom=NL
  '';

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  # Move to hardware
  services.logind.lidSwitch = "ignore";

  system.stateVersion = "22.05";
}

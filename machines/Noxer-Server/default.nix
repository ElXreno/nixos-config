{ inputs, ... }:
{
  imports = [
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.minidlna
    ./hardware-configuration.nix
    ./qbittorrent-nox.nix
    # ./wireguard.nix
  ];

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

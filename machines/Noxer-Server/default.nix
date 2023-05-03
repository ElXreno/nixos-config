{ inputs, ... }:
{
  imports = [
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.minidlna
    inputs.self.nixosProfiles.k8s-master
    ./hardware-configuration.nix
    ./qbittorrent-nox.nix
    # ./wireguard.nix
  ];

  boot.extraModprobeConfig = ''
    options ath9k btcoex_enable=0 bt_ant_diversity=0 ps_enable=0
    options cfg80211 ieee80211_regdom=NL
  '';

  security.sudo.wheelNeedsPassword = false;

  virtualisation.containerd.settings.plugins."io.containerd.grpc.v1.cri".containerd.snapshotter = "btrfs";
  services.kubernetes.kubelet.hostname = "noxer-server";
  
  services.tailscale.enable = true;

  # Move to hardware
  services.logind.lidSwitch = "ignore";

  system.stateVersion = "22.05";
}

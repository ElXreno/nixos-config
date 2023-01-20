# Upstream: https://github.com/souxd/nixos-config/blob/b9060c609ca4275ee41c1cb5d1832f5455fbcd1b/modules/nixos/networking/qbittorrent.nix
### qBittorrent-nox
# Bittorrent client, web UI exposed via :8080
# docs: https://github.com/qbittorrent/qBittorrent/wiki
# Note: you will need to set the user up manually
# (default credentials: `admin`, `adminadmin`)
{ pkgs, ... }:

{

  environment.systemPackages = [ pkgs.qbittorrent-nox ];

  # add and enable systemd unit
  systemd = {
    packages = [ pkgs.qbittorrent-nox ];
    services."qbittorrent-nox@elxreno" = {
      enable = true;
      serviceConfig = {
        Type = "simple";
        User = "elxreno";
        ExecStart = "${pkgs.qbittorrent-nox}/bin/qbittorrent-nox";
      };
      wantedBy = [ "multi-user.target" ];
    };
  };

  networking.firewall.allowedTCPPorts = [ 8080 22835 ];
  networking.firewall.allowedUDPPorts = [ 22835 ];

}
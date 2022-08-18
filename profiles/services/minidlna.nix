{ lib, ... }:
{
  networking.firewall.allowedTCPPorts = [ 8200 ];

  services.minidlna = {
    enable = true;
    settings = {
      media_dir = [ "/home/elxreno/Videos" ];
    };
  };

  # Work-around for permissions
  systemd.services.minidlna = {
    serviceConfig = {
      User = lib.mkForce "elxreno";
      Group = lib.mkForce "users";
    };
  };
}

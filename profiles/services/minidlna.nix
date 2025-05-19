{ lib, ... }:
{
  services.minidlna = {
    enable = true;
    openFirewall = true;
    settings = {
      media_dir = [ "/mnt/media" ];
      inotify = "yes";
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

{ pkgs, ... }:
{
  services = {
    tlp = {
      enable = true;
      settings = {
        START_CHARGE_THRESH_BAT0 = 80;
        STOP_CHARGE_THRESH_BAT0 = 90;
        USB_BLACKLIST = "046d:c542"; # Logitech M190
      };
    };
  };

  # Plasma will use power-profiles-daemon which conflicts with tlp
  # https://github.com/NixOS/nixpkgs/pull/175738
  # But I don't care
  services.power-profiles-daemon.enable = false;

  # systemd.services = {
  #   "tlp" = {
  #     serviceConfig = {
  #       ExecStartPre = "${pkgs.coreutils}/bin/sleep 30";
  #     };
  #   };
  # };
}

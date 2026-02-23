{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.zaopin;
in
{
  options.${namespace}.system.hardware.zaopin = {
    enable = mkEnableOption "Whether or not to manage zaopin mouse stuff.";
  };

  config = mkIf cfg.enable {
    services.udev.packages = [
      (pkgs.writeTextFile {
        name = "zaopin-udev-rules";
        destination = "/lib/udev/rules.d/70-zaopin.rules";
        text = ''
          # Zaopin ZPW MAX
          SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="3554", ATTRS{idProduct}=="f526", MODE="0660", TAG+="uaccess"

          # Zaopin ZPW MAX Dongle
          SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="3554", ATTRS{idProduct}=="f527", MODE="0660", TAG+="uaccess"
        '';
      })
    ];

    environment.etc = {
      "libinput/local-overrides.quirks" = {
        text = ''
          [Never Debounce]
          MatchUdevType=mouse
          ModelBouncingKeys=1
        '';
      };
    };
  };
}

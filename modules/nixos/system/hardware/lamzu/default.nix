{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.lamzu;
in
{
  options.${namespace}.system.hardware.lamzu = {
    enable = mkEnableOption "Whether or not to manage lamzu mouse stuff.";
  };

  config = mkIf cfg.enable {
    services.udev.packages = [
      (pkgs.writeTextFile {
        name = "lamzu-udev-rules";
        destination = "/lib/udev/rules.d/70-lamzu.rules";
        text = ''
          # Lamzu Maya X (Normal Mode)
          SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="001c", MODE="0660", TAG+="uaccess"

          # Lamzu Maya X DFU (DFU Mode)
          SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="b01c", MODE="0660", TAG+="uaccess"

          # Lamzu Maya X 8K Dongle (Normal Mode)
          SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="001e", MODE="0660", TAG+="uaccess"

          # Lamzu Maya X 8K Dongle DFU (DFU Mode)
          SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="b01e", MODE="0660", TAG+="uaccess"
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

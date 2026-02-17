{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.rk-m87;
in
{
  options.${namespace}.system.hardware.rk-m87 = {
    enable = mkEnableOption "Whether or not to manage stuff for rk-m87 keyboard.";
  };

  config = mkIf cfg.enable {
    services.udev = {
      extraRules = ''
        # USB Dongle
        SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="258a", ATTRS{idProduct}=="0150", MODE="0666", TAG+="uaccess"

        # Keyboard
        SUBSYSTEM=="hidraw", KERNEL=="hidraw*", ATTRS{idVendor}=="258a", ATTRS{idProduct}=="01a2", MODE="0666", TAG+="uaccess"
      '';
    };
  };
}

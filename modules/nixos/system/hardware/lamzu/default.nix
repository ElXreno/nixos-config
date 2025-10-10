{
  config,
  namespace,
  lib,
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
    services.udev = {
      extraRules = ''
        SUBSYSTEM=="hidraw", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="001c", TAG+="uaccess"
        SUBSYSTEM=="usb", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="001c", TAG+="uaccess"
        SUBSYSTEM=="hidraw", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="001e", TAG+="uaccess"
        SUBSYSTEM=="usb", ATTRS{idVendor}=="373e", ATTRS{idProduct}=="001e", TAG+="uaccess"
      '';
    };

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

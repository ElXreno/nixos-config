{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.silakka54;
in
{
  options.${namespace}.system.hardware.silakka54 = {
    enable = mkEnableOption "Whether or not to manage stuff for silakka54 keyboard.";
  };

  config = mkIf cfg.enable {
    environment.systemPackages = [ pkgs.vial ];
    services.udev = {
      extraRules = ''
        KERNEL=="hidraw*", SUBSYSTEM=="hidraw", ATTRS{serial}=="*vial:f64c2b3c*", ATTRS{idVendor}=="feed", ATTRS{idProduct}=="1212", MODE="0660", GROUP="users", TAG+="uaccess", TAG+="udev-acl"
      '';
    };
  };
}

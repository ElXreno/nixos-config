{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.ddc;
in
{
  options.${namespace}.system.hardware.ddc = {
    enable = mkEnableOption "controlling external monitors over DDC/CI" // {
      default = config.services.graphical-desktop.enable;
    };
  };

  config = mkIf cfg.enable {
    boot.kernelModules = [ "i2c-dev" ];
    users.groups.i2c = { };
    services.udev.packages = [ pkgs.ddcutil ];
    environment.systemPackages = [ pkgs.ddcutil ];
  };
}

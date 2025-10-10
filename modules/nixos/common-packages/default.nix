{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.common-packages;
in
{
  options.${namespace}.common-packages = {
    enable = mkEnableOption "Whether or not to provision common packages.";
  };

  config = mkIf cfg.enable {
    environment.systemPackages = with pkgs; [
      gparted
      config.boot.kernelPackages.cpupower
    ];
  };
}

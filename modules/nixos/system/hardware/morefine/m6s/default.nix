{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.morefine.m6s;
in
{
  options.${namespace}.system.hardware.morefine.m6s = {
    enable = mkEnableOption "managing MOREFINE M6S stuff";
  };

  config = mkIf cfg.enable {
    services.thermald = {
      enable = true;
      configFile = ./thermal-conf.xml;
    };

    boot = {
      extraModulePackages = [ config.boot.kernelPackages.mmio-fan ];
      kernelModules = [ "mmio_fan" ];
      blacklistedKernelModules = [ "int3403_thermal" ];
    };
  };
}

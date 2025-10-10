{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.system.hardware.cpu.amd.zenpower;
in
{
  options.${namespace}.system.hardware.cpu.amd.zenpower = {
    enable = mkEnableOption "Whether to utilize zenpower for sensors." // {
      default = config.${namespace}.system.hardware.cpu.manufacturer == "amd";
    };
  };

  config = mkIf cfg.enable {
    boot.blacklistedKernelModules = [ "k10temp" ];
    boot.extraModulePackages = [ config.boot.kernelPackages.zenpower ];
    boot.kernelModules = [ "zenpower" ];
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    mkPackageOption
    optional
    types
    ;
  cfg = config.${namespace}.system.hardware.gpu.intel;
in
{
  options.${namespace}.system.hardware.gpu.intel = {
    enable = mkEnableOption "managing Intel graphics" // {
      default = config.${namespace}.facts.gpu.intel.exists;
      defaultText = "Auto-detected from facter report.";
    };
    vaapiDriver = mkOption {
      type = types.enum [
        "iHD"
        "i965"
      ];
      default = "iHD";
      description = "VA-API driver: iHD for Broadwell and newer, i965 for older generations.";
    };
    computeRuntime = mkPackageOption pkgs "intel-compute-runtime" { nullable = true; };
    loadHuc = mkEnableOption "loading the HuC firmware for low-power encoding";
  };

  config = mkIf cfg.enable {
    hardware.graphics = {
      enable = true;
      extraPackages = [
        (if cfg.vaapiDriver == "iHD" then pkgs.intel-media-driver else pkgs.intel-vaapi-driver)
      ]
      ++ optional (cfg.computeRuntime != null) cfg.computeRuntime;
    };

    boot.kernelParams = mkIf cfg.loadHuc [ "i915.enable_guc=2" ];
  };
}

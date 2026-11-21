{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    getBin
    getExe
    mapAttrsToList
    mkEnableOption
    mkIf
    ;
  nvidia = config.${namespace}.system.hardware.gpu.nvidia;

  environment = {
    TRITON_PTXAS_BLACKWELL_PATH = getExe pkgs.${namespace}.ptxas-fp8-blockscale;
    PTXAS_FP8_FALLBACK = "${getBin pkgs.cudaPackages_13.cuda_nvcc}/bin/ptxas";
  };
in
{
  options.${namespace}.system.hardware.gpu.nvidia.fp8BlockScale.enable =
    mkEnableOption "block-scaled FP8 MMA in every Triton on GeForce Blackwell"
    // {
      default = nvidia.enable;
    };

  config = mkIf nvidia.fp8BlockScale.enable {
    environment.sessionVariables = environment;
    systemd.settings.Manager.DefaultEnvironment = mapAttrsToList (n: v: "${n}=${v}") environment;
  };
}

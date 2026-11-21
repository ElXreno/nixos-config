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
  cfg = config.${namespace}.system.hardware.gpu.amd;
in
{
  options.${namespace}.system.hardware.gpu.amd = {
    enable = mkEnableOption "managing amdgpu stuff" // {
      default = config.${namespace}.facts.gpu.amd.exists;
      defaultText = "Auto-detected from facter report.";
    };
  };

  config = mkIf cfg.enable {
    environment.sessionVariables = {
      MESA_SHADER_CACHE_MAX_SIZE = "4G";
    };

    hardware = {
      graphics = {
        enable = true;
        enable32Bit = true;
      };

      amdgpu = {
        opencl.enable = true;
        initrd.enable = true;
      };
    };
  };
}

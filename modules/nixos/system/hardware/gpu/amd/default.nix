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
    enable = mkEnableOption "Whether or not to manage amdgpu stuff." // {
      default = config.${namespace}.facts.gpu.amd.exists;
      defaultText = "Auto-detected from facter report.";
    };
  };

  config = mkIf cfg.enable {
    environment.sessionVariables = {
      RADV_PERFTEST = "gpl,nggc,transfer_queue";
      MESA_SHADER_CACHE_MAX_SIZE = "4G";
      MESA_DISK_CACHE_SINGLE_FILE = "1";
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

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
    enable = mkEnableOption "Whether or not to manage amdgpu stuff.";
  };

  config = mkIf cfg.enable {
    hardware = {
      graphics = {
        enable = true;
        enable32Bit = true;
      };

      amdgpu = {
        # Replaced via RADV in Mesa
        # amdvlk = {
        #   enable = true;
        #   support32Bit.enable = true;
        # };

        opencl.enable = true;

        initrd.enable = true;
      };
    };
  };
}

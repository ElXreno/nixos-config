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
  cfg = config.${namespace}.system.virtualisation.docker;
in
{
  options.${namespace}.system.virtualisation.docker = {
    enable = mkEnableOption "Whether or not to manage docker.";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = [
      "/var/lib/docker"
    ];

    virtualisation.docker.enable = true;

    hardware.nvidia-container-toolkit.enable = config.hardware.nvidia.enabled;
  };
}

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
  cfg = config.${namespace}.system.virtualisation.podman;
in
{
  options.${namespace}.system.virtualisation.podman = {
    enable = mkEnableOption "Whether or not to manage podman.";
  };

  config = mkIf cfg.enable {
    networking.dhcpcd.IPv6rs = false; # Not in use anyway

    virtualisation.podman.enable = true;

    hardware.nvidia-container-toolkit.enable = config.hardware.nvidia.enabled;
  };
}

{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkEnableOption mkIf;
  cfg = config.${namespace}.system.virtualisation.guest;
in
{
  options.${namespace}.system.virtualisation.guest = {
    enable = mkEnableOption "the hypervisor guest integration" // {
      default = config.${namespace}.facts.system.virtualised;
      defaultText = "config.\${namespace}.facts.system.virtualised";
    };
  };

  config = mkIf cfg.enable {
    services.qemuGuest.enable = mkIf config.hardware.facter.detected.virtualisation.qemu.enable true;
    hardware.facter.detected.graphics.enable = false;
  };
}

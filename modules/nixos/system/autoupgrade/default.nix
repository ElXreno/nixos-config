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
  cfg = config.${namespace}.system.autoupgrade;
in
{
  options.${namespace}.system.autoupgrade = {
    enable = mkEnableOption "Whether to enable autoupgrade.";
    allowReboot = mkEnableOption "Whether to allow reboot after upgrade if fresh initrd, kernel or kernel modules is available.";
  };

  config = mkIf cfg.enable {
    system.autoUpgrade = {
      enable = true;
      flake = "github:ElXreno/nixos-config";
      flags = [ "--print-build-logs" ];
      dates = "04:00";
      allowReboot = cfg.allowReboot;
      rebootWindow = {
        lower = "03:00";
        upper = "05:00";
      };
    };
  };
}

{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.morefine.m6s;
in
{
  options.${namespace}.system.hardware.morefine.m6s = {
    enable = mkEnableOption "managing MOREFINE M6S stuff";
  };

  config = mkIf cfg.enable {
    systemd.settings.Manager.RuntimeWatchdogSec = "30s";
  };
}

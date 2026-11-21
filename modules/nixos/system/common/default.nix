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
    mkOption
    types
    ;
  cfg = config.${namespace}.system.common;
in
{
  options.${namespace} = {
    primaryUser = mkOption {
      type = types.str;
      description = "Main user of this machine, the one per-user integrations target.";
    };

    system.common = {
      enable = mkEnableOption "managing common stuff" // {
        default = true;
      };
    };
  };

  config = mkIf cfg.enable {
    time.timeZone = "Europe/Minsk";

    services = {
      # I don't use that
      speechd.enable = false;
      orca.enable = false;
    };

    security.sudo = {
      execWheelOnly = true;
      extraConfig = ''
        Defaults lecture = never
      '';
    };

    systemd.settings.Manager.DefaultOOMPolicy = "continue";
  };
}

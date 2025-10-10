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
  cfg = config.${namespace}.system.common;
in
{
  options.${namespace}.system.common = {
    enable = mkEnableOption "Whether or not to manage common stuff." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    time.timeZone = "Europe/Minsk";

    i18n.defaultLocale = lib.mkDefault "en_US.UTF-8";
    console = {
      # font = "Lat2-Terminus16";
      keyMap = "us";
    };

    services = {
      # I don't use that
      speechd.enable = false;
      orca.enable = false;
    };
  };
}

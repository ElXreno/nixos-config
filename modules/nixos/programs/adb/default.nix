{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.adb;
in
{
  options.${namespace}.programs.adb = {
    enable = mkEnableOption "Whether or not to manage adb.";
  };

  config = mkIf cfg.enable {
    programs.adb.enable = true;
  };
}

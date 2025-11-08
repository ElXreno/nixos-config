{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.gamescope;
in
{
  options.${namespace}.programs.gamescope = {
    enable = mkEnableOption "Whether or not to manage gamescope.";
  };

  config = mkIf cfg.enable {
    programs.gamescope = {
      enable = true;
    };
  };
}

{
  config,
  lib,
  pkgs,
  namespace,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.linux-wallpaperengine;
in
{
  options.${namespace}.programs.linux-wallpaperengine = {
    enable = mkEnableOption "Whether to install linux-wallpaperengine.";

    package = mkOption {
      type = types.package;
      default = pkgs.linux-wallpaperengine;
    };
  };

  config = mkIf cfg.enable {
    home.packages = [ cfg.package ];
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.hyprpaper;
in
{
  options.${namespace}.programs.hyprpaper = {
    enable = mkEnableOption "Whether or not to manage hyprpaper.";
  };

  config = mkIf cfg.enable {
    services.hyprpaper = {
      enable = true;
      settings = {
        splash = false;
        wallpaper = {
          monitor = "";
          path = "${toString pkgs.${namespace}.custom-wallpaper}";
        };
      };
    };
  };
}

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
        preload = [
          "${pkgs.${namespace}.custom-wallpapers}/1398562.png"
        ];

        wallpaper = ", ${pkgs.${namespace}.custom-wallpapers}/1398562.png";
      };
    };
  };
}

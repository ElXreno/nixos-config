{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.plasma-mobile;
in
{
  options.${namespace}.desktop-environments.plasma-mobile = {
    enable = mkEnableOption "managing Plasma Mobile" // {
      default = osConfig.${namespace}.desktop-environments.plasma-mobile.enable;
    };
  };

  config = mkIf cfg.enable {
    programs.plasma = {
      enable = true;
      configFile.kwinrc.Wayland.InputMethod = "/run/current-system/sw/share/applications/org.kde.plasma.keyboard.desktop";
    };
  };
}

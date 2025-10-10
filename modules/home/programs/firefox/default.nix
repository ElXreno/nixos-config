{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption mkPackageOption;
  cfg = config.${namespace}.programs.firefox;
in
{
  options.${namespace}.programs.firefox = {
    enable = mkEnableOption "Whether or not to manage Firefox.";
    package = mkPackageOption pkgs "firefox" { };
  };

  config = mkIf cfg.enable {
    programs.firefox = {
      enable = true;
      package = cfg.package;

      languagePacks = [
        "ru"
        "en"
      ];

      profiles.default = {
        settings = {
          "widget.use-xdg-desktop-portal.file-picker" = 1;
        };
      };
    };

    xdg.mimeApps.defaultApplications = {
      # Don't abuse me by using Thunderbird by default
      "text/html" = "firefox.desktop";
      "x-scheme-handler/http" = "firefox.desktop";
      "x-scheme-handler/https" = "firefox.desktop";
      "x-scheme-handler/about" = "firefox.desktop";
      "x-scheme-handler/unknown" = "firefox.desktop";
    };
  };
}

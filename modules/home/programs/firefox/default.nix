{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkPackageOption
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.firefox;
in
{
  options.${namespace}.programs.firefox = {
    enable = mkEnableOption "Whether or not to manage Firefox.";
    package = mkPackageOption pkgs "firefox" { };

    extensions = {
      packages = mkOption {
        type = with types; listOf package;
        default = [ ];
      };

      settings = mkOption {
        type = types.raw;
        default = { };
      };
    };

    settings = mkOption {
      type = types.raw;
      default = { };
    };
  };

  config = mkIf cfg.enable {
    stylix.targets.firefox = {
      profileNames = [ "default" ];
      colorTheme.enable = true;
    };

    programs.firefox = {
      enable = true;
      inherit (cfg) package;

      languagePacks = [
        "ru"
        "en"
      ];

      profiles.default = {
        extensions = {
          packages =
            with pkgs.firefox-addons;
            [
              ublock-origin
              absolute-enable-right-click
              clearurls
            ]
            ++ cfg.extensions.packages;

          settings = {
            "uBlock0@raymondhill.net".settings = {
              selectedFilterLists = [
                "user-filters"
                "ublock-filters"
                "ublock-badware"
                "ublock-privacy"
                "ublock-quick-fixes"
                "ublock-unbreak"
                "ublock-experimental"
                "easylist"
                "adguard-generic"
                "adguard-mobile"
                "easyprivacy"
                "adguard-spyware-url"
                "block-lan"
                "urlhaus-1"
                "curben-phishing"
                "plowe-0"
                "dpollock-0"
                "fanboy-cookiemonster"
                "ublock-cookies-easylist"
                "adguard-cookies"
                "ublock-cookies-adguard"
                "fanboy-social"
                "adguard-social"
                "fanboy-thirdparty_social"
                "fanboy-ai-suggestions"
                "easylist-chat"
                "easylist-newsletters"
                "easylist-notifications"
                "easylist-annoyances"
                "adguard-mobile-app-banners"
                "adguard-other-annoyances"
                "adguard-popup-overlays"
                "adguard-widgets"
                "ublock-annoyances"
                "RUS-0"
                "RUS-1"
              ];
            };
          }
          // cfg.extensions.settings;

          force = true;
        };
        inherit (cfg) settings;
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

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
        "en-US"
      ];

      policies = {
        DisableTelemetry = true;
        DisableFirefoxStudies = true;
        DisableRemoteImprovements = true;
        DontCheckDefaultBrowser = true;
        NoDefaultBookmarks = true;
        OverrideFirstRunPage = "";
        OverridePostUpdatePage = "";

        HttpsOnlyMode = "enabled";

        EnableTrackingProtection = {
          Value = false;
          # Cryptomining = true;
          # Fingerprinting = true;
          # EmailTracking = true;
          # SuspectedFingerprinting = true;
          # Category = "strict";
        };

        GenerativeAI = {
          Chatbot = false;
          LinkPreviews = false;
        };
      };

      profiles.default = {
        isDefault = true;

        search = {
          default = "google";
          force = true;
          engines = {
            "GitHub" = {
              urls = [ { template = "https://github.com/search?q={searchTerms}&type=repositories"; } ];
              definedAliases = [ "@gh" ];
            };
            "Nix Packages" = {
              urls = [ { template = "https://search.nixos.org/packages?channel=unstable&query={searchTerms}"; } ];
              definedAliases = [ "@np" ];
            };
            "Nix Options" = {
              urls = [ { template = "https://search.nixos.org/options?channel=unstable&query={searchTerms}"; } ];
              definedAliases = [ "@no" ];
            };
            "Home Manager Options" = {
              urls = [
                { template = "https://home-manager-options.extranix.com/?query={searchTerms}&release=master"; }
              ];
              definedAliases = [ "@hm" ];
            };
            "Nixpkgs Pull Request Tracker" = {
              urls = [
                { template = "https://nixpk.gs/pr-tracker.html?pr={searchTerms}"; }
              ];
              definedAliases = [ "@npr" ];
            };
            "bing".metaData.hidden = true;
          };
        };

        settings = {
          "browser.tabs.fadeOutUnloadedTabs" = true;
          "browser.tabs.unloadOnLowMemory" = true;
          "browser.tabs.min_inactive_duration_before_unload" = 14400;

          # GPU acceleration
          "widget.dmabuf.force-enabled" = true;
          "media.hardware-video-decoding.force-enabled" = true;
          # Rendering performance
          "nglayout.initialpaint.delay" = 0;
          "nglayout.initialpaint.delay_in_oopif" = 0;

          # ECH via local dnscrypt-proxy DoH
          "network.trr.mode" = 2;
          "network.trr.uri" = "https://127.0.0.1:3000/dns-query";
          "network.trr.custom_uri" = "https://127.0.0.1:3000/dns-query";
          "network.dns.echconfig.enabled" = true;
          "network.dns.use_https_rr_as_alts" = true;

          # Accessibility — disable if not using screen readers
          "accessibility.force_disabled" = 1;

          # Faster math — uses platform libm instead of fdlibm
          "javascript.options.use_fdlibm_for_sin_cos_tan" = false;
        }
        // cfg.settings;

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

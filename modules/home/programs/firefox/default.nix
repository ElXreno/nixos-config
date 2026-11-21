{
  config,
  namespace,
  lib,
  pkgs,
  osConfig,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkPackageOption
    mkOption
    optionalAttrs
    types
    ;
  cfg = config.${namespace}.programs.firefox;

  ramMiB = osConfig.${namespace}.facts.memory.totalMiB;

  themeStyles = config.stylix.enable && (cfg.allStyles || cfg.styles != [ ]);

  userContent = pkgs.${namespace}.userstyles-base16.override {
    colors = config.lib.stylix.colors.withHashtag;
    styles = if cfg.allStyles then null else cfg.styles;
    inherit (cfg) excludeStyles styleVars;
  };
in
{
  options.${namespace}.programs.firefox = {
    enable = mkEnableOption "managing Firefox";
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

    styles = mkOption {
      type = with types; listOf str;
      default = [
        "github"
        "wiki.nixos.org"
        "reddit"
        "nixos-search"
      ];
      description = "Names of catppuccin/userstyles styles to build against the stylix palette.";
    };

    allStyles = mkOption {
      type = types.bool;
      default = true;
      description = "Whether to build every style the collection ships.";
    };

    excludeStyles = mkOption {
      type = with types; listOf str;
      default = [ ];
      description = "Styles to drop when allStyles is enabled.";
    };

    styleVars = mkOption {
      type = with types; attrsOf (attrsOf str);
      default = {
        claude.accentColor = "peach";
      };
      description = "Per-style UserCSS variable overrides, keyed by style name.";
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

        HttpsOnlyMode = "force_enabled";

        EnableTrackingProtection = {
          Locked = true;
          Category = "strict";
        };

        GenerativeAI = {
          Chatbot = false;
          LinkPreviews = false;
        };
      };

      profiles.default = {
        userContent = mkIf themeStyles userContent;

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
          "toolkit.legacyUserProfileCustomizations.stylesheets" = themeStyles;

          "browser.tabs.fadeOutUnloadedTabs" = true;
          "browser.tabs.unloadOnLowMemory" = true;
          "browser.tabs.min_inactive_duration_before_unload" = 14400000;
          "browser.low_commit_space_threshold_percent" = 20;

          # ECH via local dnscrypt-proxy DoH
          "network.trr.mode" = 3;
          "network.trr.uri" = "https://127.0.0.1:${
            toString osConfig.${namespace}.ports.dnscrypt-local-doh
          }/dns-query";
          "network.trr.excluded-domains" = osConfig.clan.core.settings.domain;

          "gfx.webrender.layer-compositor" = true;
          "gfx.content.skia-font-cache-size" = 32;
          "webgl.max-size" = 16384;

          "browser.cache.disk.metadata_memory_limit" = 16384;
          "browser.cache.jsbc_compression_level" = 3;
          "browser.cache.memory.max_entry_size" = 20480;

          "image.mem.decode_bytes_at_a_time" = 65536;

          "network.buffer.cache.size" = 65535;
          "network.buffer.cache.count" = 48;
          "network.ssl_tokens_cache_capacity" = 10240;

          "browser.cache.memory.capacity" = ramMiB * 4;
        }
        // optionalAttrs (ramMiB * 16 > 524288) {
          "media.memory_caches_combined_limit_kb" = ramMiB * 16;
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

      "application/pdf" = "firefox.desktop";
      "image/svg+xml" = "firefox.desktop";
    };
  };
}

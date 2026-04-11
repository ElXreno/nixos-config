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

  # TODO: Move this to the system config
  ramMiB =
    with builtins;
    (head (
      filter (elem: elem.type == "phys_mem")
        (head (filter (elem: elem.model == "Main Memory") osConfig.hardware.facter.report.hardware.memory))
        .resources
    )).range
    / 1024
    / 1024;
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
      package = cfg.package.override {
        # TODO: Remove after INFINITY deploy
        extraPrefs = ''
          clearPref("gfx.webrender.all");
          clearPref("widget.dmabuf.force-enabled");
          clearPref("media.hardware-video-decoding.force-enabled");
          clearPref("nglayout.initialpaint.delay");
          clearPref("nglayout.initialpaint.delay_in_oopif");
          clearPref("javascript.options.use_fdlibm_for_sin_cos_tan");
          clearPref("accessibility.force_disabled");
          clearPref("network.trr.custom_uri");
          clearPref("network.dns.echconfig.enabled");
        '';
      };

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
          "browser.tabs.min_inactive_duration_before_unload" = 14400000;
          "browser.low_commit_space_threshold_percent" = 20;

          # ECH via local dnscrypt-proxy DoH
          "network.trr.mode" = 3;
          "network.trr.uri" = "https://127.0.0.1:3000/dns-query";

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
    };
  };
}

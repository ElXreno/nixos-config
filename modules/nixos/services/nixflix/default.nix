{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.nixflix;
  vars = config.clan.core.vars.generators;
  impermanenceCfg = config.${namespace}.system.impermanence;
in
{
  options.${namespace}.services.nixflix = {
    enable = mkEnableOption "Whether or not to manage Nixflix.";
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators = {
      nixflix-sonarr = {
        files."api-key".secret = true;
        files."password".secret = true;
        runtimeInputs = [ pkgs.openssl ];
        script = ''
          openssl rand -hex 16 > "$out/api-key"
          openssl rand -base64 32 > "$out/password"
        '';
      };

      nixflix-radarr = {
        files."api-key".secret = true;
        files."password".secret = true;
        runtimeInputs = [ pkgs.openssl ];
        script = ''
          openssl rand -hex 16 > "$out/api-key"
          openssl rand -base64 32 > "$out/password"
        '';
      };

      nixflix-prowlarr = {
        files."api-key".secret = true;
        files."password".secret = true;
        runtimeInputs = [ pkgs.openssl ];
        script = ''
          openssl rand -hex 16 > "$out/api-key"
          openssl rand -base64 32 > "$out/password"
        '';
      };

      nixflix-jellyfin = {
        files."api-key".secret = true;
        files."admin-password".secret = true;
        runtimeInputs = [ pkgs.openssl ];
        script = ''
          openssl rand -hex 16 > "$out/api-key"
          openssl rand -base64 32 > "$out/admin-password"
        '';
      };

      nixflix-rutracker = {
        prompts = {
          username = {
            description = "RuTracker username";
            type = "hidden";
            persist = true;
          };
          password = {
            description = "RuTracker password";
            type = "hidden";
            persist = true;
          };
        };
      };

      nixflix-qbittorrent = {
        files."password".secret = true;
        files."password-pbkdf2".secret = false;
        runtimeInputs = [
          pkgs.openssl
          pkgs.python3
        ];
        script = ''
          openssl rand -base64 32 > "$out/password"
          python3 -c "
          import hashlib, os, base64
          password = open('$out/password').read().strip().encode()
          salt = os.urandom(16)
          dk = hashlib.pbkdf2_hmac('sha512', password, salt, 100000)
          print(f'@ByteArray({base64.b64encode(salt).decode()}:{base64.b64encode(dk).decode()})')
          " | tr -d '\n' > "$out/password-pbkdf2"
        '';
      };
    };

    systemd.services.jellyfin-dlna-plugin = {
      description = "Install Jellyfin DLNA plugin";
      before = [ "jellyfin.service" ];
      requiredBy = [ "jellyfin.service" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        User = "jellyfin";
        Group = "media";
      };
      script =
        let
          dlnaPlugin = pkgs.fetchzip {
            url = "https://repo.jellyfin.org/files/plugin/dlna/dlna_10.0.0.0.zip";
            hash = "sha256-EAEnwOA/NzyP3R8JjQLtzYWP62nIYdM3eDPvbpB+kqE=";
            stripRoot = false;
          };
        in
        ''
          PLUGIN_DIR="${config.nixflix.jellyfin.dataDir}/plugins/DLNA"
          mkdir -p "$PLUGIN_DIR"
          cp -rn ${dlnaPlugin}/* "$PLUGIN_DIR"/
          chmod -R u+w "$PLUGIN_DIR"
        '';
    };

    users.groups.media = lib.mkForce {
      gid = config.nixflix.globals.gids.media;
      members = [ "elxreno" ];
    };

    networking.firewall = {
      allowedTCPPorts = [
        config.nixflix.jellyfin.network.internalHttpPort
        config.nixflix.torrentClients.qbittorrent.torrentingPort
      ];
      allowedUDPPorts = [
        1900 # SSDP (Jellyfin auto-discovery)
        7359 # Jellyfin client discovery
        config.nixflix.torrentClients.qbittorrent.torrentingPort # DHT / μTP
      ];
    };

    ${namespace} = {
      services.postgresql.enable = true;
      system.impermanence.directories = [
        "/var/cache/jellyfin"
        {
          directory = "/var/lib/jellyfin";
          user = "jellyfin";
          group = "media";
          mode = "0755";
        }
        "/var/lib/qBittorrent"
      ];
    };

    nixflix = {
      enable = true;
      mediaDir = "${impermanenceCfg.defaultPersistentPath}/var/lib/nixflix/media";
      downloadsDir = "${impermanenceCfg.defaultPersistentPath}/var/lib/nixflix/downloads";
      stateDir = "${impermanenceCfg.defaultPersistentPath}/var/lib/nixflix/.state";

      postgres.enable = true;

      sonarr = {
        enable = true;
        config = {
          apiKey._secret = vars.nixflix-sonarr.files."api-key".path;
          hostConfig.password._secret = vars.nixflix-sonarr.files."password".path;
        };
      };

      radarr = {
        enable = true;
        config = {
          apiKey._secret = vars.nixflix-radarr.files."api-key".path;
          hostConfig.password._secret = vars.nixflix-radarr.files."password".path;
        };
      };

      prowlarr = {
        enable = true;
        config = {
          apiKey._secret = vars.nixflix-prowlarr.files."api-key".path;
          hostConfig.password._secret = vars.nixflix-prowlarr.files."password".path;
          indexers = [
            {
              name = "RuTracker.org";
              enable = true;
              username._secret = vars.nixflix-rutracker.files."username".path;
              password._secret = vars.nixflix-rutracker.files."password".path;
              "torrentBaseSettings.seedRatio" = 5;
            }
            {
              name = "Knaben";
              enable = true;
              "torrentBaseSettings.seedRatio" = 2;
            }
            {
              name = "TorrentsCSV";
              enable = true;
              "torrentBaseSettings.seedRatio" = 2;
            }
          ];
        };
      };

      recyclarr = {
        enable = true;
        cleanupUnmanagedProfiles = {
          enable = true;
          managedProfiles = [
            "[SQP] SQP-2"
            "WEB-2160p (Alternative)"
          ];
        };
        config.radarr.radarr = {
          quality_profiles = [
            {
              trash_id = "c3933358ba2356bafc41524f81471069"; # [SQP] SQP-2
              reset_unmatched_scores.enabled = true;
            }
          ];
          custom_formats = [
            {
              trash_ids = [ "4b900e171accbfb172729b63323ea8ca" ]; # MULTi
              assign_scores_to = [
                {
                  trash_id = "c3933358ba2356bafc41524f81471069"; # [SQP] SQP-2
                  score = 100;
                }
              ];
            }
          ];
        };
        config.sonarr.sonarr = {
          quality_profiles = [
            {
              trash_id = "dfa5eaae7894077ad6449169b6eb03e0"; # WEB-2160p (Alternative)
              reset_unmatched_scores.enabled = true;
            }
          ];
          custom_formats = [
            {
              trash_ids = [ "7ba05c6e0e14e793538174c679126996" ]; # MULTi
              assign_scores_to = [
                {
                  trash_id = "dfa5eaae7894077ad6449169b6eb03e0"; # WEB-2160p (Alternative)
                  score = 100;
                }
              ];
            }
          ];
        };
      };

      jellyfin = {
        enable = true;
        apiKey._secret = vars.nixflix-jellyfin.files."api-key".path;
        users.admin = {
          policy.isAdministrator = true;
          password._secret = vars.nixflix-jellyfin.files."admin-password".path;
        };

        system.trickplayOptions = {
          enableHwAcceleration = true;
          enableHwEncoding = true;
        };

        encoding = {
          hardwareAccelerationType = "vaapi";
          allowHevcEncoding = true;
          allowAv1Encoding = false;
          enableTonemapping = true;
          enableThrottling = true;
          enableSegmentDeletion = true;
          hardwareDecodingCodecs = [
            "h264"
            "hevc"
            "mpeg2video"
            "vc1"
            "vp8"
            "vp9"
          ];
        };
      };

      downloadarr.qbittorrent = {
        sequentialOrder = true;
        firstAndLast = true;
      };

      torrentClients.qbittorrent = {
        enable = true;
        torrentingPort = 56670;
        password._secret = vars.nixflix-qbittorrent.files."password".path;
        serverConfig = {
          BitTorrent.Session.DisableAutoTMMByDefault = false;
          Preferences.WebUI = {
            Username = "admin";
            Password_PBKDF2 = vars.nixflix-qbittorrent.files."password-pbkdf2".value;
          };
        };
      };
    };
  };
}

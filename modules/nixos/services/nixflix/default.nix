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

      recyclarr.enable = true;

      jellyfin = {
        enable = true;
        openFirewall = true;
        apiKey._secret = vars.nixflix-jellyfin.files."api-key".path;
        network.enableUPnP = true;
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
          allowAv1Encoding = true;
          enableTonemapping = true;
          enableThrottling = true;
          enableSegmentDeletion = true;
          hardwareDecodingCodecs = [
            "h264"
            "hevc"
            "mpeg2video"
            "vc1"
            "vp9"
            "av1"
          ];
        };
      };

      downloadarr.qbittorrent = {
        sequentialOrder = true;
        firstAndLast = true;
      };

      torrentClients.qbittorrent = {
        enable = true;
        openFirewall = true;
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

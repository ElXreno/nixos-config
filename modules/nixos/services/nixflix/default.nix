{
  config,
  options,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkMerge mkEnableOption;
  cfg = config.${namespace}.services.nixflix;
  vars = config.clan.core.vars.generators;
  ports = config.${namespace}.ports;
  impermanenceCfg = config.${namespace}.system.impermanence;

  basePath = if impermanenceCfg.enable then impermanenceCfg.defaultPersistentPath else "";

  arrServices = [
    "sonarr"
    "radarr"
    "prowlarr"
  ];

  credentials = passwordFile: {
    files."api-key".secret = true;
    files.${passwordFile}.secret = true;
    runtimeInputs = [ pkgs.openssl ];
    script = ''
      openssl rand -hex 16 > "$out/api-key"
      openssl rand -base64 32 > "$out/${passwordFile}"
    '';
  };

  serveAt = host: comment: port: {
    inherit comment;
    tags = [ "tag:nixflix" ];
    endpoints."tcp:443" = "http://${host}:${toString port}";
  };
  serve = serveAt "localhost";

  tunnel = "airvpn";

  arrDatabases = lib.concatMap (
    name:
    let
      inherit (config.nixflix.${name}.settings) postgres;
    in
    [
      (lib.nameValuePair postgres.mainDb {
        owner = postgres.user;
        backup = true;
      })
      (lib.nameValuePair postgres.logDb {
        owner = postgres.user;
        backup = true;
      })
    ]
  ) arrServices;

  jellyfinDb = "${config.nixflix.stateDir}/jellyfin/data/jellyfin.db";
in
{
  options.${namespace}.services.nixflix = {
    enable = mkEnableOption "managing Nixflix";
    retain = mkEnableOption "backing up its data while the service is off" // {
      default = options.${namespace}.services.nixflix.enable.highestPrio < 1500;
      defaultText = "Whether this host sets `enable` at all, even to false.";
    };
  };

  config = mkMerge [
    (mkIf (cfg.enable || cfg.retain) {
      nixflix.stateDir = "${basePath}/var/lib/nixflix/.state";

      ${namespace} = {
        services = {
          postgresql = {
            enable = true;
            databases = lib.listToAttrs arrDatabases;
          };
          kopia.backup = [
            {
              path = config.nixflix.stateDir;
              ignore = [
                "/postgres"
                "/*/logs"
                "/*/MediaCover"
                "/*/Sentry"
                "/*/*.pid"
                "/prowlarr/Definitions"
                "/jellyfin/.cache"
                "/jellyfin/log"
                "/jellyfin/metadata"
                "/jellyfin/data/trickplay"
                "/jellyfin/data/jellyfin.db*"
              ];
            }
            {
              path = "/nixflix/jellyfin";
              fileName = "jellyfin.db.sql";
              command = "${pkgs.util-linux}/bin/runuser -u ${config.nixflix.jellyfin.user} -- ${lib.getExe pkgs.sqlite} -readonly ${lib.escapeShellArg jellyfinDb} .dump";
            }
          ];
        };
        system.impermanence.directories = [
          "/var/cache/jellyfin"
          "/var/lib/qBittorrent"
        ];
      };
    })
    (mkIf cfg.enable {
      clan.core.vars.generators = {
        nixflix-sonarr = credentials "password";
        nixflix-radarr = credentials "password";
        nixflix-prowlarr = credentials "password";
        nixflix-jellyfin = credentials "admin-password";

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

      # workaround: nixflix merges the uid into its mkForce wrapper, so it never applies
      users.users.qbittorrent = lib.mkForce { uid = config.nixflix.globals.uids.qbittorrent; };

      ${namespace} = {
        ports = {
          radarr = config.nixflix.radarr.config.hostConfig.port;
          sonarr = config.nixflix.sonarr.config.hostConfig.port;
          prowlarr = config.nixflix.prowlarr.config.hostConfig.port;
          jellyfin = config.nixflix.jellyfin.network.internalHttpPort;
          qbittorrent-webui = config.nixflix.torrentClients.qbittorrent.webuiPort;
          qbittorrent-torrenting = config.nixflix.torrentClients.qbittorrent.torrentingPort;
        };
        system.networking = {
          routes.confine.qbittorrent = tunnel;
          firewall.public = {
            jellyfin.tcp = [ ports.jellyfin ];
            jellyfin-discovery.udp = [ 7359 ];
          };
        };
        tailscale.serve = {
          "svc:radarr" = serve "Radarr movie manager" ports.radarr;
          "svc:sonarr" = serve "Sonarr TV show manager" ports.sonarr;
          "svc:prowlarr" = serve "Prowlarr indexer manager" ports.prowlarr;
          "svc:jellyfin" = serve "Jellyfin media server" ports.jellyfin;
          "svc:qbittorrent" =
            serveAt config.vpnNamespaces.${tunnel}.namespaceAddress "qBittorrent download client"
              ports.qbittorrent-webui;
        };
      };

      nixflix = {
        enable = true;
        mediaDir = "${basePath}/var/lib/nixflix/media";
        downloadsDir = "${basePath}/var/lib/nixflix/downloads";
        mediaUsers = [ config.${namespace}.primaryUser ];

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
            enableDecodingColorDepth10HevcRext = true;
            enableIntelLowPowerH264HwEncoder = true;
            enableIntelLowPowerHevcHwEncoder = true;
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

        vpn = {
          enable = true;
          namespace = tunnel;
          wgConfFile = config.${namespace}.system.networking.tunnels.${tunnel}.configFile;
          allowedIngress = [ ];
          allowedTCPPorts = [ config.nixflix.torrentClients.qbittorrent.torrentingPort ];
          allowedUDPPorts = [ config.nixflix.torrentClients.qbittorrent.torrentingPort ];
        };

        torrentClients.qbittorrent = {
          enable = true;
          torrentingPort = 43629;
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
    })
  ];
}

{
  config,
  options,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkMerge
    mkEnableOption
    mkOption
    types
    ;
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

  inherit (cfg) tunnel;

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

  russianAudio = {
    name = "Russian audio";
    score = 5000;
    minScore = 4000;
    patterns = {
      tokens = ''\b(rus|russian)\b(?![ ._-]?subs?\b)'';
      voiceover = ''\b(mvo|dvo|avo|ru[ ._-]?dub)\b'';
      cyrillic = "^(?!.*[іїєґў]).*[а-яё]";
      studios = ''\b(lostfilm|hdrezka|tvshows|newstudio|jaskier|alexfilm|kubik|red[ ._-]?head[ ._-]?sound|le[ ._-]?production|dragon[ ._-]?money|baibako|coldfilm|novafilm|amedia|selezen|generalfilm|kinozal|rutracker|nnmclub)\b'';
    };
  };

  russianAudioSync = pkgs.writeShellApplication {
    name = "nixflix-russian-audio";
    runtimeInputs = [
      pkgs.curl
      pkgs.jq
    ];
    text = ''
      name=${lib.escapeShellArg russianAudio.name}
      format=$(jq -n --arg name "$name" --argjson patterns ${lib.escapeShellArg (builtins.toJSON russianAudio.patterns)} '{
        name: $name,
        includeCustomFormatWhenRenaming: false,
        specifications: [$patterns | to_entries[] | {
          name: .key,
          implementation: "ReleaseTitleSpecification",
          negate: false,
          required: false,
          fields: [{name: "value", value: .value}]
        }]
      }')

      headers=$(mktemp)
      trap 'rm -f "$headers"' EXIT

      sync() {
        local url=$1 id
        printf 'X-Api-Key: %s\nContent-Type: application/json\n' "$(cat "$2")" > "$headers"
        local api=(curl -fsS --retry 30 --retry-delay 2 --retry-all-errors -H "@$headers")

        id=$("''${api[@]}" "$url/api/v3/customformat" | jq --arg name "$name" '.[] | select(.name == $name) | .id')
        if [ -n "$id" ]; then
          "''${api[@]}" -X PUT "$url/api/v3/customformat/$id" -d "$(jq --argjson id "$id" '. + {id: $id}' <<<"$format")" >/dev/null
        else
          id=$("''${api[@]}" -X POST "$url/api/v3/customformat" -d "$format" | jq .id)
        fi

        "''${api[@]}" "$url/api/v3/qualityprofile" | jq -c '.[]' | while read -r profile; do
          "''${api[@]}" -X PUT "$url/api/v3/qualityprofile/$(jq .id <<<"$profile")" -d "$(jq --argjson id "$id" \
            --argjson score ${toString russianAudio.score} '.formatItems |= map(if .format == $id then .score = $score else . end)' <<<"$profile")" >/dev/null
        done
      }

      sync http://127.0.0.1:${toString ports.sonarr} ${vars.nixflix-sonarr.files."api-key".path}
      sync http://127.0.0.1:${toString ports.radarr} ${vars.nixflix-radarr.files."api-key".path}
    '';
  };
in
{
  options.${namespace}.services.nixflix = {
    enable = mkEnableOption "managing Nixflix";
    tunnel = mkOption {
      type = types.str;
      default = "airvpn";
      description = "Tunnel qBittorrent is confined to.";
    };
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

      systemd.services.nixflix-russian-audio = {
        description = "Keep the Russian audio custom format in Sonarr and Radarr";
        wantedBy = [
          "multi-user.target"
          "recyclarr.service"
        ];
        wants = [
          "sonarr.service"
          "radarr.service"
        ];
        after = [
          "sonarr.service"
          "radarr.service"
          "recyclarr.service"
        ];
        serviceConfig = {
          Type = "oneshot";
          ExecStart = lib.getExe russianAudioSync;
          PrivateTmp = true;
        };
      };

      ${namespace} = {
        ports = {
          radarr = config.nixflix.radarr.config.hostConfig.port;
          sonarr = config.nixflix.sonarr.config.hostConfig.port;
          prowlarr = config.nixflix.prowlarr.config.hostConfig.port;
          jellyfin = config.nixflix.jellyfin.network.internalHttpPort;
          qbittorrent-webui = config.nixflix.torrentClients.qbittorrent.webuiPort;
          qbittorrent-torrenting = config.nixflix.torrentClients.qbittorrent.torrentingPort;
        };
        system.networking.routes.confine.qbittorrent = tunnel;
        tailscale.serveNode."tcp:8920".backend = "http://localhost:${toString ports.jellyfin}";
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
                name = "roj";
                schemaName = "Generic Torznab";
                enable = true;
                baseUrl = "https://roj.${config.clan.core.settings.domain}/torznab";
                apiPath = "/api";
                "torrentBaseSettings.preferMagnetUrl" = true;
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
                min_format_score = russianAudio.minScore;
                reset_unmatched_scores = {
                  enabled = true;
                  except = [ russianAudio.name ];
                };
              }
            ];
          };
          config.sonarr.sonarr = {
            quality_profiles = [
              {
                trash_id = "dfa5eaae7894077ad6449169b6eb03e0"; # WEB-2160p (Alternative)
                min_format_score = russianAudio.minScore;
                reset_unmatched_scores = {
                  enabled = true;
                  except = [ russianAudio.name ];
                };
              }
            ];
            custom_formats = [
              {
                trash_ids = [ "ae575f95ab639ba5d15f663bf019e3e8" ]; # Language: Not Original
                assign_scores_to = [
                  {
                    trash_id = "dfa5eaae7894077ad6449169b6eb03e0"; # WEB-2160p (Alternative)
                    score = -1000;
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

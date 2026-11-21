{
  config,
  inputs,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    concatStringsSep
    length
    mkEnableOption
    mkForce
    mkIf
    mkOption
    optional
    optionalAttrs
    types
    ;
  cfg = config.${namespace}.services.roj;
  ports = config.${namespace}.ports;
  netns = config.vpnNamespaces.airvpn;
  reviewCap = "elxreno.com/cap/roj";
  stores = [ "clickhouse.service" ] ++ optional cfg.web.engine "manticore.service";
  afterStores = {
    wants = stores;
    after = stores;
  };
in
{
  imports = [ inputs.roj.nixosModules.default ];

  options.${namespace}.services.roj = {
    enable = mkEnableOption "the roj DHT crawler";
    autostart = mkEnableOption "the roj DHT crawler at boot";
    web = {
      enable = mkEnableOption "the roj search pages";
      serve = mkEnableOption "the roj search pages in the tailnet as svc:roj";
      sharePort = mkOption {
        type = types.nullOr types.port;
        default = null;
        description = "Port of the machine's own tailnet name that serves the pages to users the machine is shared with.";
      };
      posters = mkEnableOption "posters and descriptions from TMDB on the roj search pages";
      owners = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "Tailnet logins that may open the review screen.";
      };
      catalogue = mkOption {
        type = types.nullOr types.str;
        default = "daily";
        description = "When to refresh the title catalogue and rebuild the search table, as a systemd calendar expression.";
      };
      classifier = mkEnableOption "scoring the new torrents before each refresh, with the model files put into /var/lib/private/roj-classifier";
      engine = mkEnableOption "full-text search of the roj pages in a local Manticore Search server";
      reindexMemory = mkOption {
        type = types.ints.positive;
        default = 15000000000;
        description = "Bytes a rebuild of the search table may hold in memory before it spills to the disk.";
      };
    };
    forwardedPort = mkOption {
      type = types.nullOr types.port;
      default = null;
      description = "Port the VPN provider forwards, used for the DHT and for incoming peers.";
    };
    scalingFactor = mkOption {
      type = types.ints.positive;
      default = 50;
      description = "Concurrency multiplier of the crawler pipeline.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        roj-metrics = 3340;
      }
      // optionalAttrs cfg.web.enable { roj-web = 8480; }
      // optionalAttrs (cfg.web.enable && cfg.web.sharePort != null) { roj-share = cfg.web.sharePort; }
      // optionalAttrs (cfg.forwardedPort != null) { roj-dht = cfg.forwardedPort; };
      tailscale.serveNode = mkIf (cfg.web.enable && cfg.web.sharePort != null) {
        "tcp:${toString cfg.web.sharePort}" = {
          backend = "http://localhost:${toString ports.roj-web}";
          acceptAppCaps = [ reviewCap ];
          shared = true;
        };
      };
      tailscale.serve."svc:roj" = mkIf (cfg.web.enable && cfg.web.serve) {
        comment = "roj torrent search";
        acceptAppCaps = [ reviewCap ];
        endpoints."tcp:443" = "http://localhost:${toString ports.roj-web}";
      };
      services = {
        clickhouse = {
          enable = true;
          allowedNetworks = [ netns.namespaceAddress ];
        };
        manticore.enable = mkIf (cfg.web.enable && cfg.web.engine) true;
        monitoring.agent.targets.roj = {
          host = netns.namespaceAddress;
          port = ports.roj-metrics;
        };
      };
      system = {
        networking.routes.confine.roj-crawler = "airvpn";
        impermanence.directories = [
          {
            directory = "/var/lib/private/roj-crawler";
            user = "nobody";
            group = "nogroup";
            mode = "0700";
          }
        ]
        ++ optional cfg.web.classifier {
          directory = "/var/lib/private/roj-classifier";
          user = "nobody";
          group = "nogroup";
          mode = "0700";
        };
      };
    };

    networking.firewall.interfaces."airvpn-br".allowedTCPPorts = [ ports.clickhouse-native ];

    services.roj-crawler = {
      enable = true;
      package = pkgs.roj-crawler;
      metricsAddr = ":${toString ports.roj-metrics}";
      geoipDatabase = "${pkgs.dbip-country-lite}/share/dbip/dbip-country-lite.mmdb";
      clickhouse.addr = "${netns.bridgeAddress}:${toString ports.clickhouse-native}";
      environment = {
        DHT_CRAWLER_SCALING_FACTOR = toString cfg.scalingFactor;
        DHT_CRAWLER_SAVE_FILES_THRESHOLD = "1000000";
        ROJ_SAMPLE_CONCURRENCY = "4000";
        ROJ_SAMPLE_SCHEDULE = "on";
        ROJ_SAMPLE_PROBE_SECONDS = "900";
        ROJ_FAILED_TTL_HOURS = "2";
        ROJ_FAILED_SMOOTH = "on";
        ROJ_TRACKER_POLICY = "off";
        ROJ_RESCRAPE = "on";
        ROJ_DHT_REFUSE_SILENT = "on";
        ROJ_DHT_IGNORE_IDS = "d9d0b383eebeaf16e1cfb9f7a1adc197fa1a9f8d,181f8a8b9398a7a8abb1b7bac3cbd7dae6eaf3fe";
        ROJ_DHT_DETECT_ENFORCE = "family,storm,scanner";
        ROJ_COVERAGE = "on";
      }
      // optionalAttrs (cfg.forwardedPort != null) {
        DHT_SERVER_PORT = toString cfg.forwardedPort;
        ROJ_INBOUND_PEERS = "on";
        ROJ_INBOUND_DHT = "on";
        ROJ_INBOUND_DHT_PPS = "200";
        ROJ_INBOUND_DHT_WORKERS = "160";
        ROJ_INBOUND_MAX_CONNS = "15000";
        ROJ_INBOUND_UNKNOWN = "on";
        ROJ_INBOUND_UNKNOWN_SCRAPE = "on";
        ROJ_INBOUND_BARREN = "on";
        ROJ_INBOUND_MSE_REFUSE_PERCENT = "100";
        ROJ_INBOUND_HANDSHAKE_MS = "2000";
        ROJ_INBOUND_FETCH_SECONDS = "60";
        ROJ_INBOUND_BUDGET_MB = "1024";
        ROJ_INBOUND_RESUME = "on";
        ROJ_INBOUND_DIALECT = "on";
        ROJ_DIAL_DIALECT = "on";
        ROJ_INBOUND_SIGHTINGS = "on";
        ROJ_INBOUND_DELAYED_ACK = "on";
        ROJ_INBOUND_RESET = "on";
      }
      // optionalAttrs (length netns.interfaces > 1) {
        ROJ_DHT_INTERFACES = concatStringsSep "," netns.interfaces;
      };
    };

    systemd.services.roj-crawler = {
      wantedBy = mkIf (!cfg.autostart) (mkForce [ ]);
      wants = [ "clickhouse.service" ];
      after = [ "clickhouse.service" ];
    };

    clan.core.vars.generators.roj-tmdb = mkIf (cfg.web.enable && cfg.web.posters) {
      prompts.token = {
        description = "TMDB API read access token from https://www.themoviedb.org/settings/api";
        type = "hidden";
        persist = true;
      };
    };

    services.roj-web = mkIf cfg.web.enable {
      enable = true;
      package = pkgs.roj-crawler;
      listen = "127.0.0.1:${toString ports.roj-web}";
      clickhouse.addr = "127.0.0.1:${toString ports.clickhouse-native}";
      manticore = mkIf cfg.web.engine "127.0.0.1:${toString ports.manticore-mysql}";
      tmdbTokenFile = mkIf cfg.web.posters config.clan.core.vars.generators.roj-tmdb.files.token.path;
      ownerCapability = mkIf (cfg.web.serve || cfg.web.sharePort != null) reviewCap;
      inherit (cfg.web)
        owners
        catalogue
        classifier
        reindexMemory
        ;
    };

    systemd.services.roj-web = mkIf cfg.web.enable afterStores;
    systemd.services.roj-web-reindex = mkIf cfg.web.enable afterStores;
    systemd.services.roj-catalogue = mkIf cfg.web.enable afterStores;
    systemd.services.roj-index = mkIf (cfg.web.enable && cfg.web.classifier) afterStores;
    systemd.services.roj-web-mirror = mkIf (cfg.web.enable && cfg.web.engine) afterStores;

    vpnNamespaces.airvpn = {
      allowedEgress = [ "${netns.bridgeAddress}/32" ];
      portMappings = [
        {
          from = ports.roj-metrics;
          to = ports.roj-metrics;
        }
      ];
      openVPNPorts = mkIf (cfg.forwardedPort != null) [
        {
          port = cfg.forwardedPort;
          protocol = "both";
        }
      ];
      extraRuleset = mkIf (cfg.forwardedPort != null) (
        let
          syn = "tcp dport ${toString cfg.forwardedPort} tcp flags & (syn | ack) == syn";
          # a BitTorrent handshake with the peer id -BM0001-
          crawler = "tcp dport ${toString cfg.forwardedPort} @ih,0,64 0x13426974546f7272 @ih,384,64 0x2d424d303030312d";
        in
        ''
          table inet roj {
            set crawlers4 {
              type ipv4_addr
              flags dynamic,timeout
              timeout 30m
              size 262144
            }
            set crawlers6 {
              type ipv6_addr
              flags dynamic,timeout
              timeout 30m
              size 262144
            }
            chain peers {
              type filter hook prerouting priority raw - 10; policy accept;
              ${syn} ip saddr @crawlers4 drop
              ${syn} ip6 saddr @crawlers6 drop
              ${crawler} update @crawlers4 { ip saddr }
              ${crawler} update @crawlers6 { ip6 saddr }
            }
          }
        ''
      );
    };
  };
}

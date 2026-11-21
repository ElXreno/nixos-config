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
    mkOption
    types
    ;
  cfg = config.${namespace}.services.monitoring.agent;
  inherit (config.${namespace}) ports;
  inherit (config.networking) hostName;

  zfsEnabled = config.${namespace}.services.monitoring.prometheus.exporters.zfs.enable;

  jobs = lib.mapAttrsToList (
    job_name: target:
    {
      inherit job_name;
      static_configs = [
        {
          targets = [ "http://${target.host}:${toString target.port}${target.path}" ];
          labels.instance = hostName;
        }
      ];
    }
    // lib.optionalAttrs (target.interval != null) { scrape_interval = target.interval; }
  ) cfg.targets;
in
{
  options.${namespace}.services.monitoring.agent = {
    enable = mkEnableOption "shipping this machine's metrics and journal to the monitoring server";

    metricsUrl = mkOption {
      type = types.str;
      example = "http://monitoring.example.ts.net:8428";
      description = "Base URL of the VictoriaMetrics instance receiving remote writes.";
    };

    logsUrl = mkOption {
      type = types.str;
      example = "http://monitoring.example.ts.net:9428";
      description = "Base URL of the VictoriaLogs instance receiving the journal.";
    };

    journal.enable = mkEnableOption "shipping the journal to VictoriaLogs" // {
      default = true;
    };

    targets = mkOption {
      type = types.attrsOf (
        types.submodule {
          options = {
            host = mkOption {
              type = types.str;
              default = "127.0.0.1";
              description = "Address the metrics endpoint listens on.";
            };
            port = mkOption {
              type = types.port;
              description = "Port of the metrics endpoint.";
            };
            path = mkOption {
              type = types.str;
              default = "/metrics";
              description = "HTTP path of the metrics endpoint.";
            };
            interval = mkOption {
              type = types.nullOr types.str;
              default = null;
              example = "60s";
              description = "Scrape interval, when it differs from the global one.";
            };
          };
        }
      );
      default = { };
      description = "Metrics endpoints on this machine, keyed by job name. Each service registers its own.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports.vmagent = 8429;
      services.monitoring = {
        prometheus.exporters.enable = true;
        agent.targets.vmagent.port = ports.vmagent;
      };
      system.impermanence.directories = [
        {
          directory = "/var/cache/private/vmagent";
          user = "nobody";
          group = "nogroup";
          mode = "0700";
        }
      ]
      ++ lib.optional config.services.vector.enable {
        directory = "/var/lib/private/vector";
        user = "nobody";
        group = "nogroup";
        mode = "0700";
      };
    };

    services.vmagent = {
      enable = true;
      remoteWrite.url = "${cfg.metricsUrl}/api/v1/write";
      extraArgs = [
        "-httpListenAddr=127.0.0.1:${toString ports.vmagent}"
        "-remoteWrite.label=host=${hostName}"
        "-remoteWrite.maxDiskUsagePerURL=1GB"
      ];
      prometheusConfig = {
        global.scrape_interval = "10s";
        scrape_configs = jobs;
      };
    };

    systemd.services.zpool-influxdb-push = mkIf zfsEnabled {
      after = [ "vmagent.service" ];
      serviceConfig.Type = "oneshot";
      script = ''
        ${config.boot.zfs.package}/libexec/zfs/zpool_influxdb --sum-histogram-buckets \
          | ${lib.getExe pkgs.curl} --silent --show-error --retry 5 --retry-connrefused --data-binary @- \
            "http://127.0.0.1:${toString ports.vmagent}/write?extra_label=instance=${hostName}"
      '';
    };

    systemd.timers.zpool-influxdb-push = mkIf zfsEnabled {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "30s";
        OnUnitActiveSec = "15s";
      };
    };

    services.vector = mkIf cfg.journal.enable {
      enable = true;
      journaldAccess = true;
      settings = {
        sources.journal = {
          type = "journald";
          current_boot_only = false;
          since_now = true;
        };

        transforms.journal_fields = {
          type = "remap";
          inputs = [ "journal" ];
          source = ''
            ._HOSTNAME = del(.host)
            del(.source_type)
            realtime, err = to_int(.__REALTIME_TIMESTAMP)
            if err == null {
              .timestamp = from_unix_timestamp!(realtime * 1000, unit: "nanoseconds")
            }
            if exists(.PRIORITY) {
              levels = {
                "0": "emerg",
                "1": "alert",
                "2": "critical",
                "3": "error",
                "4": "warning",
                "5": "notice",
                "6": "info",
                "7": "debug"
              }
              priority = to_string(.PRIORITY) ?? ""
              level = get(levels, [priority]) ?? null
              .level = if level == null { priority } else { level }
            }
            . = filter(.) -> |key, _value| { !starts_with(key, "__") }
          '';
        };

        sinks.journal_vlogs = {
          type = "elasticsearch";
          inputs = [ "journal_fields" ];
          endpoints = [ "${cfg.logsUrl}/insert/elasticsearch/" ];
          api_version = "v8";
          compression = "gzip";
          healthcheck.enabled = false;
          acknowledgements.enabled = true;
          query = {
            _msg_field = "message";
            _time_field = "timestamp";
            _stream_fields = "_MACHINE_ID,_HOSTNAME,_SYSTEMD_UNIT";
          };
        };
      };
    };

    systemd.services.vector.after = mkIf cfg.journal.enable [ "tailscaled.service" ];
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.monitoring.grafana;
  ports = config.${namespace}.ports;
  adminPassword = config.clan.core.vars.generators.grafana-admin.files.password.path;

  withHostVar =
    name: metric: bare: src:
    pkgs.runCommand "${name}-dashboard.json" { } ''
      ${lib.getExe pkgs.jq} --arg metric ${metric} --argjson bare ${lib.escapeShellArg (builtins.toJSON bare)} \
        -f ${./dashboards/host-var.jq} ${src} > $out
    '';

  appCap = "elxreno.com/cap/grafana";
  capField =
    field:
    ''"~\"${lib.escapeRegex appCap}\":\[\x7b[^]]*\"${field}\":\"(?P<value>[^\"]+)\"" "''${value}"'';
in
{
  options.${namespace}.services.monitoring.grafana = {
    enable = mkEnableOption "managing grafana";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        grafana = 3001;
        grafana-tailscale-auth = 3002;
      };
      services.monitoring.agent.targets.grafana.port = ports.grafana;
      system.impermanence.directories = [
        {
          directory = "/var/lib/grafana";
          user = "grafana";
          group = "grafana";
          mode = "0700";
        }
      ];
      tailscale.serve."svc:grafana" = {
        comment = "Grafana";
        acceptAppCaps = [ appCap ];
        endpoints."tcp:443" = "http://localhost:${toString ports.grafana-tailscale-auth}";
      };
    };

    services.caddy = {
      enable = true;
      virtualHosts.grafana-tailscale-auth = {
        hostName = "http://:${toString ports.grafana-tailscale-auth}";
        extraConfig = ''
          bind 127.0.0.1
          map {header.Tailscale-App-Capabilities} {grafana_login} {
            ${capField "login"}
            default ""
          }
          map {header.Tailscale-App-Capabilities} {grafana_role} {
            ${capField "role"}
            default ""
          }
          reverse_proxy 127.0.0.1:${toString ports.grafana} {
            header_up X-WEBAUTH-USER {grafana_login}
            header_up X-WEBAUTH-ROLE {grafana_role}
          }
        '';
      };
    };

    clan.core.vars.generators.grafana-secret = {
      files.secret_key = {
        secret = true;
        owner = "grafana";
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl rand -base64 32 | tr -d '\n' > "$out/secret_key"
      '';
    };

    clan.core.vars.generators.grafana-admin = {
      files.password = {
        secret = true;
        owner = "grafana";
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl rand -base64 24 | tr -d '\n' > "$out/password"
      '';
    };

    systemd.services.grafana.serviceConfig.ExecStartPre = lib.mkAfter [
      "-${pkgs.writeShellScript "grafana-admin-password" ''
        ${lib.getExe config.services.grafana.package} cli --homepath ${config.services.grafana.dataDir} \
          admin reset-admin-password --password-from-stdin < ${adminPassword}
      ''}"
    ];

    services.grafana = {
      enable = true;
      settings = {
        server = {
          http_addr = "127.0.0.1";
          http_port = ports.grafana;
          enable_gzip = true;
        };

        security = {
          admin_email = "admin@example.com";
          admin_password = "$__file{${adminPassword}}";
          secret_key = "$__file{${config.clan.core.vars.generators.grafana-secret.files.secret_key.path}}";
        };
        analytics.reporting_enabled = false;

        "auth.proxy" = {
          enabled = true;
          header_name = "X-WEBAUTH-USER";
          header_property = "username";
          headers = "Role:X-WEBAUTH-ROLE";
          auto_sign_up = true;
          whitelist = "127.0.0.1, ::1";
        };
      };

      provision = {
        dashboards.settings.providers = [
          {
            name = "Overview";
            options.path = "/etc/grafana-dashboards";
          }
        ];

        datasources.settings = {
          apiVersion = 1;
          datasources = [
            {
              name = "VictoriaMetrics";
              type = "victoriametrics-metrics-datasource";
              access = "proxy";
              url = "http://127.0.0.1:${toString ports.victoriametrics}";
              isDefault = true;
            }

            {
              name = "VictoriaMetrics (Prometheus)";
              type = "prometheus";
              access = "proxy";
              url = "http://127.0.0.1:${toString ports.victoriametrics}";
              isDefault = false;
            }

            {
              name = "VictoriaLogs";
              uid = "victorialogs";
              type = "victoriametrics-logs-datasource";
              access = "proxy";
              url = "http://127.0.0.1:${toString ports.victorialogs}";
              isDefault = false;
            }
          ];
        };
      };

      declarativePlugins = with pkgs.grafanaPlugins; [
        grafana-metricsdrilldown-app

        prometheus
        victoriametrics-metrics-datasource
        victoriametrics-logs-datasource
      ];
    };

    environment.etc = {
      node-exporter-full = {
        source = pkgs.fetchurl {
          url = "https://github.com/rfmoz/grafana-dashboards/raw/ff9173b154a5f9809263c31e72e4ed5ba453864d/prometheus/node-exporter-full.json";
          hash = "sha256-P326XW/h/z4yydUQ0Jm2e+YtDWkh3Rl17ZlANVjS35s=";
        };
        target = "grafana-dashboards/node-exporter-full.json";
        group = "grafana";
        user = "grafana";
      };

      dnscrypt-proxy = {
        source = ./dashboards/dnscrypt-proxy.json;
        target = "grafana-dashboards/dnscrypt-proxy.json";
        group = "grafana";
        user = "grafana";
      };

      nvidia-gpu =
        let
          nvidia-dashboard-original =
            pkgs.prometheus-nvidia-gpu-exporter.src + "/docs/grafana/dashboard.json";
          nvidia-dashboard-patched = pkgs.runCommand "nvidia-gpu-dashboard-patched.json" { } ''
            original_json=$(cat ${nvidia-dashboard-original})
            ${lib.getExe pkgs.jq} '.templating.list += [{
              "description": "",
              "label": "Datasource",
              "name": "DS_PROMETHEUS",
              "options": [],
              "query": "prometheus",
              "refresh": 1,
              "regex": "",
              "type": "datasource"
            }]' <<< "$original_json" > $out
          '';
        in
        {
          source = nvidia-dashboard-patched;
          target = "grafana-dashboards/nvidia-gpu.json";
          group = "grafana";
          user = "grafana";
        };

      sing-box = {
        source = pkgs.writeText "sing-box.json" (builtins.toJSON (import ./dashboards/_sing-box.nix));
        target = "grafana-dashboards/sing-box.json";
        group = "grafana";
        user = "grafana";
      };

      zfs = {
        source = withHostVar "zfs" "zfs_pool_health" [
          "zfs_pool_deduplication_ratio"
          "node_zfs_arc_c"
          "node_zfs_arc_c_max"
          "node_zfs_arc_data_size"
          "node_zfs_arc_metadata_size"
          "node_zfs_arc_mfu_size"
          "node_zfs_arc_mru_size"
          "node_zfs_arc_size"
          "node_zfs_arc_hits"
          "node_zfs_arc_misses"
          "zfs_pool_allocated_bytes"
          "zfs_pool_size_bytes"
          "zfs_pool_fragmentation_ratio"
          "zfs_pool_free_bytes"
          "zfs_pool_health"
        ] ./dashboards/zfs.json;
        target = "grafana-dashboards/zfs.json";
        group = "grafana";
        user = "grafana";
      };

      zerofs = {
        source = withHostVar "zerofs" "zerofs_used_bytes" [ ] ./dashboards/zerofs.json;
        target = "grafana-dashboards/zerofs.json";
        group = "grafana";
        user = "grafana";
      };

      zrepl = {
        source =
          withHostVar "zrepl" "zrepl_start_time" [ "zrepl_endpoint_abstractions_cache_entry_count" ]
            (
              pkgs.runCommand "zrepl-dashboard-patched.json" { } ''
                ${lib.getExe pkgs.jq} '.templating.list += [{
                  "description": "",
                  "label": "Datasource",
                  "name": "DS_PROMETHEUS",
                  "options": [],
                  "query": "prometheus",
                  "refresh": 1,
                  "regex": "",
                  "type": "datasource"
                }]' ${pkgs.zrepl.src}/dist/grafana/grafana-prometheus-zrepl.json > $out
              ''
            );
        target = "grafana-dashboards/zrepl.json";
        group = "grafana";
        user = "grafana";
      };

      roj = {
        source = withHostVar "roj" "roj_dht_crawler_persisted_total" [ ] ./dashboards/roj.json;
        target = "grafana-dashboards/roj.json";
        group = "grafana";
        user = "grafana";
      };
    };
  };
}

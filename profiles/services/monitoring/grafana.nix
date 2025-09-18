{
  config,
  lib,
  pkgs,
  ...
}:
let
  prometheusExporters = config.services.prometheus.exporters;
in
{
  services.grafana = {
    enable = true;
    settings = {
      server = {
        http_addr = "127.0.0.1";
        http_port = 3000;
        enable_gzip = true;
      };

      security.admin_email = "admin@example.com";
      analytics.reporting_enabled = false;
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
            url = "http://127.0.0.1:8428";
            isDefault = true;
          }

          {
            name = "VictoriaMetrics (Prometheus)";
            type = "prometheus";
            access = "proxy";
            url = "http://127.0.0.1:8428";
            isDefault = false;
          }

          {
            name = "VictoriaLogs";
            type = "victoriametrics-logs-datasource";
            access = "proxy";
            url = "http://127.0.0.1:9428";
            isDefault = false;
          }
        ];
      };
    };

    declarativePlugins = with pkgs.grafanaPlugins; [
      grafana-metricsdrilldown-app

      victoriametrics-metrics-datasource
      victoriametrics-logs-datasource
    ];
  };

  environment.etc = {
    node-exporter-full = lib.mkIf prometheusExporters.node.enable {
      source = pkgs.fetchurl {
        url = "https://github.com/rfmoz/grafana-dashboards/raw/ff9173b154a5f9809263c31e72e4ed5ba453864d/prometheus/node-exporter-full.json";
        hash = "sha256-P326XW/h/z4yydUQ0Jm2e+YtDWkh3Rl17ZlANVjS35s=";
      };
      target = "grafana-dashboards/node-exporter-full.json";
      group = "grafana";
      user = "grafana";
    };

    dnscrypt-proxy = lib.mkIf config.services.dnscrypt-proxy.enable {
      source = ./dashboards/dnscrypt-proxy.json;
      target = "grafana-dashboards/dnscrypt-proxy.json";
      group = "grafana";
      user = "grafana";
    };

    nvidia-gpu = lib.mkIf prometheusExporters.nvidia-gpu.enable (
      let
        nvidia-dashboard-original = pkgs.prometheus-nvidia-gpu-exporter.src + "/grafana/dashboard.json";
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
      }
    );
  };
}

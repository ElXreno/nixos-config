{ config, lib, ... }:
let
  prometheusExporters = config.services.prometheus.exporters;

  nodeExporter = prometheusExporters.node;
  zfsExporter = prometheusExporters.zfs;
  smartctlExporter = prometheusExporters.smartctl;
  nvidiaExporter = prometheusExporters.nvidia-gpu;

  victoriaLogs = config.services.victorialogs;
  dnscrypt = config.services.dnscrypt-proxy;
in
{
  services.victoriametrics = {
    enable = true;

    retentionPeriod = "14d";

    prometheusConfig = {
      global.scrape_interval = "10s";
      scrape_configs = [
        {
          job_name = "victoriametrics";

          static_configs = [
            {
              targets = [ "http://127.0.0.1:8428/metrics" ];
            }
          ];
        }
        (lib.mkIf victoriaLogs.enable {
          job_name = "victorialogs";

          static_configs = [
            {
              targets = [ "${victoriaLogs.listenAddress}" ];
            }
          ];
        })
        (lib.mkIf nodeExporter.enable {
          job_name = "node";

          static_configs = [
            {
              targets = [ "127.0.0.1:${toString nodeExporter.port}" ];
              labels.type = "node";
            }
          ];
        })
        (lib.mkIf zfsExporter.enable {
          job_name = "zfs";

          static_configs = [
            {
              targets = [ "http://127.0.0.1:${toString zfsExporter.port}/metrics" ];
            }
          ];
        })
        (lib.mkIf smartctlExporter.enable {
          job_name = "smartctl";

          scrape_interval = smartctlExporter.maxInterval;
          static_configs = [
            {
              targets = [ "http://127.0.0.1:${toString smartctlExporter.port}/metrics" ];
            }
          ];
        })
        (lib.mkIf nvidiaExporter.enable {
          job_name = "nvidia-gpu";

          static_configs = [
            {
              targets = [ "http://127.0.0.1:${toString nvidiaExporter.port}/metrics" ];
            }
          ];
        })
        (lib.mkIf dnscrypt.enable {
          job_name = "dnscrypt-proxy";

          static_configs = [
            {
              targets = [ "http://${dnscrypt.settings.monitoring_ui.listen_address}/metrics" ];
            }
          ];
        })
      ];
    };
  };
}

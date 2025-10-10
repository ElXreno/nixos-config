{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.monitoring.victorialogs;
in
{
  options.${namespace}.services.monitoring.victorialogs = {
    enable = mkEnableOption "Whether or not to manage victorialogs.";
  };

  config = mkIf cfg.enable {
    services.victorialogs = {
      enable = true;
      extraOptions = [
        "-retentionPeriod=14d"
      ];
    };

    environment.systemPackages = [ pkgs.goflow2 ];
    networking.firewall.allowedUDPPorts = [ 2055 ];

    services.vector = {
      enable = true;
      settings = {
        sources = {
          netflow_file = {
            type = "file";
            include = [ "/var/log/netflow.jsonl" ];
            read_from = "beginning";
            ignore_checkpoints = true;
          };
        };

        transforms.parse_netflow = {
          type = "remap";
          inputs = [ "netflow_file" ];
          source = ''
            .message = string!(.message)
            parsed, err = parse_json(.message)
            if err == null && is_object(parsed) {
              . = merge!(., parsed)
            }
          '';
        };

        sinks = {
          vlogs = {
            type = "elasticsearch";
            inputs = [ "parse_netflow" ];
            endpoints = [ "http://localhost:9428/insert/elasticsearch/" ];
            api_version = "v8";
            compression = "gzip";
            healthcheck.enabled = false;
            query = {
              _msg_field = "message";
              _time_field = "time_received_ns";
              _stream_fields = "src_addr,src_port,dst_addr,dst_port,proto";
            };
          };
        };
      };
    };

    services.journald.upload = {
      enable = true;
      settings.Upload.URL = "http://localhost:9428/insert/journald";
    };
  };
}

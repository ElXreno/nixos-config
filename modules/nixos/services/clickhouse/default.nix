{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkEnableOption
    mkIf
    mkOption
    types
    ;
  cfg = config.${namespace}.services.clickhouse;
  ports = config.${namespace}.ports;
in
{
  options.${namespace}.services.clickhouse = {
    enable = mkEnableOption "the ClickHouse server";
    allowedNetworks = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = "Networks besides the loopback that the default user may connect from.";
    };
    nativePort = mkOption {
      type = types.port;
      default = 9000;
      description = "Port of the native protocol.";
    };
    httpPort = mkOption {
      type = types.port;
      default = 8123;
      description = "Port of the HTTP interface.";
    };
    memoryRatio = mkOption {
      type = types.numbers.between 0.05 0.9;
      default = 0.25;
      description = "Share of its resident plus the available memory the server may use; ClickHouse recomputes the limit as other processes grow.";
    };
    maxMemory = mkOption {
      type = types.nullOr types.ints.positive;
      default = null;
      description = "Bytes the server may use at most, on top of memoryRatio.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        clickhouse-native = cfg.nativePort;
        clickhouse-http = cfg.httpPort;
      };
      system.impermanence.directories = [
        {
          directory = "/var/lib/clickhouse";
          user = "clickhouse";
          group = "clickhouse";
          mode = "0750";
        }
      ];
    };

    services.clickhouse = {
      enable = true;
      serverConfig = {
        listen_host = "::";
        tcp_port = ports.clickhouse-native;
        http_port = ports.clickhouse-http;
        max_server_memory_usage_to_ram_ratio = cfg.memoryRatio;
      }
      // lib.optionalAttrs (cfg.maxMemory != null) { max_server_memory_usage = cfg.maxMemory; };
      usersConfig.users.default.networks = {
        "@replace" = "replace";
        ip = [
          "127.0.0.1"
          "::1"
        ]
        ++ cfg.allowedNetworks;
      };
    };

    systemd.services.clickhouse = {
      serviceConfig.CPUWeight = 20;
      restartTriggers = [
        config.environment.etc."clickhouse-server/config.d/100-nixos-module-config.yaml".source
        config.environment.etc."clickhouse-server/users.d/100-nixos-module-config.yaml".source
      ];
    };
  };
}

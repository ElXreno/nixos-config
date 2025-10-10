{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.atticd;

  baseDomain = "elxreno.com";
  cacheEndpoint = "cache.${baseDomain}";
in
{
  options.${namespace}.services.atticd = {
    enable = mkEnableOption "Whether or not to manage atticd.";
  };

  config = mkIf cfg.enable {
    ${namespace}.services = {
      postgresql.enable = true;
      nginx = {
        enable = true;
        virtualHosts = {
          "${cacheEndpoint}" = {
            locations."/" = {
              proxyPass = "http://localhost:18080";
            };
          };
        };
      };
    };

    environment.systemPackages = with pkgs; [ attic-client ];

    services.atticd = {
      enable = true;
      environmentFile = config.sops.secrets."attic/env".path;

      settings = {
        listen = "[::]:18080";

        database.url = "postgresql://atticd?host=/run/postgresql";

        chunking = {
          nar-size-threshold = 64 * 1024; # 64 KiB
          min-size = 16 * 1024; # 16 KiB
          avg-size = 64 * 1024; # 64 KiB
          max-size = 256 * 1024; # 256 KiB
        };

        garbage-collection = {
          default-retention-period = "3 months";
        };
      };
    };

    services.postgresql = {
      ensureDatabases = [ "atticd" ];
      ensureUsers = [
        {
          name = "atticd";
          ensureDBOwnership = true;
        }
      ];
    };

    sops.secrets."attic/env" = { };
  };
}

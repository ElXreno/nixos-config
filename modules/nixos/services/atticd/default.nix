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
    clan.core.vars.generators.atticd = {
      files."env".secret = true;

      script = ''
        ATTIC_SERVER_TOKEN_RS256_SECRET_BASE64=$(openssl genrsa -traditional 4096 | base64 -w0)
        cat > "$out/env" << EOF
        ATTIC_SERVER_TOKEN_RS256_SECRET_BASE64=$ATTIC_SERVER_TOKEN_RS256_SECRET_BASE64
        EOF
      '';

      runtimeInputs = with pkgs; [
        openssl
      ];
    };

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
      environmentFile = config.clan.core.vars.generators.atticd.files.env.path;

      settings = {
        listen = "[::]:18080";

        api-endpoint = "https://cache.elxreno.com:443/";

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
  };
}

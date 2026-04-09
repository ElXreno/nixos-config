{
  config,
  lib',
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkMerge
    mkOption
    mkEnableOption
    types
    ;
  cfg = config.${namespace}.services.atticd;

  baseDomain = "elxreno.com";
  cacheEndpoint = "cache.${baseDomain}";

  atticdSettings = {
    listen = "127.0.0.1:18080";

    api-endpoint = "https://cache.elxreno.com:443/";

    database.url = "postgresql://atticd?host=/run/postgresql";

    storage = {
      type = "local";
      path = "/var/lib/atticd/storage";
    };

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

  atticdConfigFile = (pkgs.formats.toml { }).generate "atticd-config.toml" atticdSettings;

  mintToken = pkgs.writeShellApplication {
    name = "attic-mint-token";
    runtimeInputs = with pkgs; [ attic-server ];
    text = ''
      if [ "$#" -lt 1 ]; then
        echo "usage: attic-mint-token <jwt-key-file> [atticadm make-token args...]" >&2
        exit 64
      fi

      key_file="$1"
      shift

      ATTIC_SERVER_TOKEN_RS256_SECRET_BASE64="$(cat "$key_file")" \
        atticadm -f ${atticdConfigFile} make-token "$@" \
        | tr -d '\n'
    '';
  };
in
{
  options.${namespace}.services.atticd = {
    enable = mkEnableOption "Whether or not to manage atticd.";

    configFile = mkOption {
      type = types.path;
      readOnly = true;
      internal = true;
      default = atticdConfigFile;
    };

    mintToken = mkOption {
      type = types.package;
      readOnly = true;
      internal = true;
      default = mintToken;
    };
  };

  config = mkMerge [
    {
      clan.core.vars.generators.attic-jwt-key = {
        share = true;
        files.key-base64 = {
          secret = true;
          deploy = false;
        };

        # Rotate every 180 days; cascades to all dependents.
        validation = lib'.mkRotationBucket 180;

        script = ''
          openssl genrsa -traditional 4096 | base64 -w0 > "$out/key-base64"
        '';

        runtimeInputs = with pkgs; [
          openssl
        ];
      };

      clan.core.vars.generators.attic-ci-token = {
        share = true;
        dependencies = [ "attic-jwt-key" ];
        files.token = {
          secret = true;
          deploy = false;
        };

        # Rotate with the JWT key — old tokens are useless once it changes.
        validation = lib'.mkRotationBucket 180;

        runtimeInputs = [ mintToken ];

        script = ''
          attic-mint-token "$in/attic-jwt-key/key-base64" \
            --sub "ci@github-actions" \
            --validity "200 days" \
            --pull "common" \
            --push "common" \
            > "$out/token"
        '';
      };
    }
    (mkIf cfg.enable {
      clan.core.vars.generators.atticd = {
        dependencies = [ "attic-jwt-key" ];
        files."env" = {
          secret = true;
          restartUnits = [ "atticd.service" ];
        };

        script = ''
          cat > "$out/env" << EOF
          ATTIC_SERVER_TOKEN_RS256_SECRET_BASE64=$(cat "$in/attic-jwt-key/key-base64")
          EOF
        '';
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
        settings = atticdSettings;
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
    })
  ];
}

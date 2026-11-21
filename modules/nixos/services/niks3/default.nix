{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkEnableOption
    mkIf
    mkMerge
    mkOption
    types
    ;
  cfg = config.${namespace}.services.niks3;
  ports = config.${namespace}.ports;
  vars = config.clan.core.vars.generators;

  port = 5751;
  host = "${cfg.server}.${config.clan.core.settings.domain}";
  secret = {
    secret = true;
    owner = config.services.niks3.user;
  };

  tokenGenerator = file: {
    share = true;
    files.token = file;
    runtimeInputs = [ pkgs.openssl ];
    script = ''
      openssl rand -base64 48 | tr -d '\n' > "$out/token"
    '';
  };
in
{
  options.${namespace}.services.niks3 = {
    enable = mkEnableOption "the niks3 binary cache on a local Garage bucket, reachable inside the tailnet";
    push.enable = mkEnableOption "uploading everything this host builds to the niks3 cache";
    server = mkOption {
      type = types.str;
      default = "bimba";
      description = "Tailnet host name of the machine running the cache.";
    };
    url = mkOption {
      type = types.str;
      readOnly = true;
      default = "http://${host}:${toString port}";
      description = "Address of the cache inside the tailnet, both substituter and push endpoint.";
    };
    publicKey = mkOption {
      type = types.str;
      readOnly = true;
      default = builtins.readFile ../../../../vars/shared/niks3-signing-key/public/value;
      description = "Key the cache signs with, as clients have to trust it.";
    };
    bucket = mkOption {
      type = types.str;
      default = "nix-cache";
      description = "Garage bucket holding the cache.";
    };
    capacity = mkOption {
      type = types.str;
      example = "500GB";
      description = "Capacity advertised for the Garage node.";
    };
  };

  config = mkMerge [
    (mkIf cfg.push.enable {
      clan.core.vars.generators.niks3-api-token = mkIf (!cfg.enable) (tokenGenerator {
        secret = true;
      });

      ${namespace}.system.impermanence.directories = [
        {
          directory = "/var/lib/private/niks3-hook";
          user = "nobody";
          group = "nogroup";
          mode = "0700";
        }
      ];

      services.niks3-auto-upload = {
        enable = true;
        serverUrl = cfg.url;
        authTokenFile = vars.niks3-api-token.files.token.path;
      };
    })
    (mkIf cfg.enable {
      ${namespace} = {
        ports.niks3 = port;
        services = {
          postgresql = {
            enable = true;
            databases.niks3 = { };
          };
          garage = {
            enable = true;
            inherit (cfg) capacity;
            buckets.${cfg.bucket} = {
              accessKeyFile = vars.niks3-s3.files.access-key.path;
              secretKeyFile = vars.niks3-s3.files.secret-key.path;
            };
          };
        };
      };

      clan.core.vars.generators = {
        niks3-s3 = {
          files.access-key = secret;
          files.secret-key = secret;
          runtimeInputs = [ pkgs.openssl ];
          script = ''
            printf 'GK%s' "$(openssl rand -hex 12)" > "$out/access-key"
            openssl rand -hex 32 | tr -d '\n' > "$out/secret-key"
          '';
        };
        niks3-api-token = tokenGenerator secret;
        niks3-signing-key = {
          share = true;
          files.key = secret;
          files.public.secret = false;
          runtimeInputs = [ pkgs.nix ];
          script = ''
            nix --extra-experimental-features nix-command key generate-secret --key-name niks3-1 > "$out/key"
            nix --extra-experimental-features nix-command key convert-secret-to-public < "$out/key" | tr -d '\n' > "$out/public"
          '';
        };
      };

      services.niks3 = {
        enable = true;
        httpAddr = "[::]:${toString ports.niks3}";
        database.createLocally = false;
        s3 = {
          endpoint = "127.0.0.1:${toString ports.garage-s3}";
          publicUrl = "http://${host}:${toString ports.garage-s3}";
          inherit (cfg) bucket;
          inherit (config.${namespace}.services.garage) region;
          useSSL = false;
          bucketLookup = "path";
          accessKeyFile = vars.niks3-s3.files.access-key.path;
          secretKeyFile = vars.niks3-s3.files.secret-key.path;
        };
        apiTokenFile = vars.niks3-api-token.files.token.path;
        signKeyFiles = [ vars.niks3-signing-key.files.key.path ];
        cacheUrl = cfg.url;
        readProxy.enable = true;
        gc.enable = false;
      };

      systemd.services.niks3 = {
        after = [
          "postgresql.target"
          "garage-bootstrap.service"
        ];
        requires = [
          "postgresql.target"
          "garage-bootstrap.service"
        ];
      };
    })
  ];
}

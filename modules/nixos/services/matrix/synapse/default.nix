{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkMerge;
  cfg = config.${namespace}.services.matrix;
  ports = config.${namespace}.ports;

  baseDomain = cfg.serverName;
  matrixHost = "matrix.${baseDomain}";
  authHost = "auth.${baseDomain}";

  wellKnownServer = builtins.toJSON {
    "m.server" = "${matrixHost}:443";
  };

  wellKnownClient = builtins.toJSON (
    {
      "m.homeserver" = {
        base_url = "https://${matrixHost}";
      };
      "org.matrix.msc2965.authentication" = {
        issuer = "https://${authHost}/";
        account = "https://${authHost}/account";
      };
    }
    // cfg.wellKnownClientExtra
  );

  wellKnownSupport = builtins.toJSON {
    contacts = [
      {
        matrix_id = "@elxreno:${baseDomain}";
        email_address = "elxreno@gmail.com";
        role = "m.role.admin";
      }
    ];
  };

  wellKnown = pkgs.linkFarm "matrix-well-known" {
    ".well-known/matrix/server" = pkgs.writeText "server" wellKnownServer;
    ".well-known/matrix/client" = pkgs.writeText "client" wellKnownClient;
    ".well-known/matrix/support" = pkgs.writeText "support" wellKnownSupport;
  };
in
{
  config = mkMerge [
    (mkIf (cfg.enable || cfg.retain) {
      ${namespace} = {
        services.kopia.backup = [ { path = "/var/lib/matrix-synapse"; } ];
        system.impermanence.directories = [
          "/var/lib/matrix-synapse"
        ];
      };
    })
    (mkIf cfg.enable {
      services.caddy.virtualHosts = {
        "${baseDomain}".extraConfig = ''
          handle /.well-known/matrix/* {
            root * ${wellKnown}
            header Content-Type application/json
            header Access-Control-Allow-Origin *
            file_server
          }
        '';
        "${matrixHost}".extraConfig = ''
          @synapse path_regexp ^(/_matrix|/_synapse/client)
          handle @synapse {
            reverse_proxy 127.0.0.1:${toString ports.synapse}
          }
          handle {
            redir https://${baseDomain}{uri} 301
          }
        '';
      };

      ${namespace} = {
        ports = {
          synapse = 8008;
          synapse-metrics = 9000;
        };
        services.monitoring.agent.targets.synapse = {
          port = ports.synapse-metrics;
          path = "/_synapse/metrics";
        };
        services.caddy.enable = true;

      };

      clan.core.vars.generators.matrix-synapse-signing-key = {
        files.signing-key = {
          secret = true;
          restartUnits = [ "matrix-synapse.service" ];
        };

        runtimeInputs = [ pkgs.matrix-synapse ];

        script = ''
          generate_signing_key -o "$out/signing-key"
        '';
      };

      systemd.services.matrix-synapse = {
        after = [ "postgresql-databases-init.service" ];
        requires = [ "postgresql-databases-init.service" ];

        serviceConfig = {
          LoadCredential = [
            "mas-shared-secret:${config.clan.core.vars.generators.matrix-mas-shared-secret.files.secret.path}"
          ];

          ExecStartPre = lib.mkBefore [
            (
              "+"
              + (pkgs.writeShellScript "matrix-synapse-deploy-signing-key" ''
                install -m 0600 -o matrix-synapse -g matrix-synapse \
                  "${config.clan.core.vars.generators.matrix-synapse-signing-key.files.signing-key.path}" \
                  /var/lib/matrix-synapse/homeserver.signing.key
              '')
            )
          ];
        };
      };

      services.matrix-synapse = {
        enable = true;
        withJemalloc = true;

        settings = {
          server_name = baseDomain;
          public_baseurl = "https://${matrixHost}/";
          report_stats = false;

          listeners = [
            {
              port = ports.synapse;
              tls = false;
              type = "http";
              x_forwarded = true;
              bind_addresses = [ "127.0.0.1" ];
              resources = [
                {
                  names = [
                    "client"
                    "federation"
                  ];
                  compress = false;
                }
              ];
            }
            {
              port = ports.synapse-metrics;
              tls = false;
              type = "http";
              bind_addresses = [ "127.0.0.1" ];
              resources = [
                {
                  names = [ "metrics" ];
                  compress = false;
                }
              ];
            }
          ];

          enable_metrics = true;
          enable_registration = false;

          database = {
            name = "psycopg2";
            args = {
              cp_min = 5;
              cp_max = 10;
              keepalives_idle = 10;
              keepalives_interval = 10;
              keepalives_count = 3;
            };
          };

          max_upload_size = "50M";
          max_image_pixels = "32M";
          dynamic_thumbnails = false;

          url_preview_enabled = true;
          url_preview_ip_range_blacklist = [
            "127.0.0.0/8"
            "10.0.0.0/8"
            "172.16.0.0/12"
            "192.168.0.0/16"
            "100.64.0.0/10"
            "fd7a:115c:a1e0::/48"
            "192.0.0.0/24"
            "169.254.0.0/16"
            "192.88.99.0/24"
            "198.18.0.0/15"
            "192.0.2.0/24"
            "198.51.100.0/24"
            "203.0.113.0/24"
            "224.0.0.0/4"
            "::1/128"
            "fe80::/10"
            "fc00::/7"
            "2001:db8::/32"
            "ff00::/8"
            "fec0::/10"
          ];

          media_retention = {
            local_media_lifetime = "3y";
            remote_media_lifetime = "30d";
          };

          matrix_authentication_service = {
            enabled = true;
            endpoint = "http://127.0.0.1:${toString ports.mas}/";
            secret_path = "/run/credentials/matrix-synapse.service/mas-shared-secret";
          };

          experimental_features = {
            msc3266_enabled = true;
          };
        };
      };
    })
  ];
}

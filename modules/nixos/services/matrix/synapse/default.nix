{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.matrix.synapse;

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

  wellKnownLocation = body: {
    extraConfig = ''
      default_type application/json;
      add_header Access-Control-Allow-Origin '*' always;
      return 200 '${body}';
    '';
  };
in
{
  options.${namespace}.services.matrix.synapse = {
    enable = mkEnableOption "Whether to manage Matrix Synapse homeserver.";

    serverName = mkOption {
      type = types.str;
      default = "elxreno.com";
      description = ''
        The Matrix `server_name`. This is the suffix on every Matrix ID
        (`@user:<server_name>`) and is permanent — changing it breaks all
        existing user IDs and federation.
      '';
    };

    wellKnownClientExtra = mkOption {
      type = types.attrs;
      default = { };
      description = ''
        Extra attributes merged into the `/.well-known/matrix/client` JSON.
        Used by voice/video modules to advertise RTC foci, etc.
      '';
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services = {
        postgresql.enable = true;
        matrix = {
          mas.enable = true;
          coturn.enable = true;
          livekit.enable = true;
          lk-jwt-service.enable = true;
          element-call.enable = true;
        };
        nginx = {
          enable = true;
          virtualHosts = {
            "${baseDomain}" = {
              locations = {
                "= /.well-known/matrix/server" = wellKnownLocation wellKnownServer;
                "= /.well-known/matrix/client" = wellKnownLocation wellKnownClient;
                "= /.well-known/matrix/support" = wellKnownLocation wellKnownSupport;
              };
            };
            "${matrixHost}" = {
              locations = {
                "~ ^(/_matrix|/_synapse/client)" = {
                  proxyPass = "http://127.0.0.1:8008";
                  extraConfig = ''
                    proxy_request_buffering off;
                  '';
                };
                "/" = {
                  return = "301 https://${baseDomain}$request_uri";
                };
              };
            };
          };
        };
      };

      system.impermanence.directories = [
        "/var/lib/matrix-synapse"
      ];
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

    clan.core.vars.generators.matrix-mas-shared-secret = {
      files.secret = {
        secret = true;
        restartUnits = [
          "matrix-synapse.service"
          "matrix-authentication-service.service"
        ];
      };

      runtimeInputs = [ pkgs.openssl ];

      script = ''
        openssl rand -hex 32 | tr -d '\n' > "$out/secret"
      '';
    };

    services.postgresql.ensureUsers = [
      { name = "matrix-synapse"; }
      { name = "matrix-authentication-service"; }
    ];

    systemd.services.matrix-postgres-init = {
      description = "Create Matrix PostgreSQL databases with C locale";
      after = [ "postgresql.target" ];
      requires = [ "postgresql.target" ];
      before = [
        "matrix-synapse.service"
        "matrix-authentication-service.service"
      ];
      wantedBy = [ "multi-user.target" ];

      path = [ config.services.postgresql.package ];

      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        User = "postgres";
        Group = "postgres";
      };

      script = ''
        set -eu

        ensure_db() {
          local name="$1" owner="$2"
          if [ "$(psql -tAc "SELECT 1 FROM pg_database WHERE datname = '$name'")" != "1" ]; then
            createdb \
              --owner="$owner" \
              --template=template0 \
              --encoding=UTF8 \
              --locale=C \
              "$name"
          fi
        }

        ensure_db matrix-synapse matrix-synapse
        ensure_db matrix-authentication-service matrix-authentication-service
      '';
    };

    systemd.services.matrix-synapse = {
      after = [ "matrix-postgres-init.service" ];
      requires = [ "matrix-postgres-init.service" ];

      serviceConfig = {
        LoadCredential = [
          "signing-key:${config.clan.core.vars.generators.matrix-synapse-signing-key.files.signing-key.path}"
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
            port = 8008;
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
            port = 9000;
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
          endpoint = "http://127.0.0.1:8081/";
          secret_path = "/run/credentials/matrix-synapse.service/mas-shared-secret";
        };

        experimental_features = {
          msc3266_enabled = true;
        };
      };
    };
  };
}

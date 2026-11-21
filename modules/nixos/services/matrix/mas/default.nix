{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf getExe;
  cfg = config.${namespace}.services.matrix;
  ports = config.${namespace}.ports;
  vars = config.clan.core.vars.generators;
  masCfg = config.services.matrix-authentication-service;

  baseDomain = cfg.serverName;
  authHost = "auth.${baseDomain}";
  publicBase = "https://${authHost}/";

  credential = name: "\${CREDENTIALS_DIRECTORY}/${name}";

  masCliWrapper = pkgs.writeShellScriptBin "mas-cli" ''
    exec sudo -u matrix-authentication-service \
      ${getExe masCfg.package} \
      --config=/run/matrix-authentication-service/config.yaml \
      "$@"
  '';
in
{
  config = mkIf cfg.enable {
    environment.systemPackages = [ masCliWrapper ];

    clan.core.vars.generators.matrix-mas-encryption = {
      files.encryption = {
        secret = true;
        restartUnits = [ "matrix-authentication-service.service" ];
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl rand -hex 32 | tr -d '\n' > "$out/encryption"
      '';
    };

    clan.core.vars.generators.matrix-mas-rsa-key = {
      files.rsa = {
        secret = true;
        restartUnits = [ "matrix-authentication-service.service" ];
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -out "$out/rsa"
      '';
    };

    clan.core.vars.generators.matrix-mas-ec-key = {
      files.ec-prime256v1 = {
        secret = true;
        restartUnits = [ "matrix-authentication-service.service" ];
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl genpkey \
          -algorithm EC \
          -pkeyopt ec_paramgen_curve:P-256 \
          -pkeyopt ec_param_enc:named_curve \
          -out "$out/ec-prime256v1"
      '';
    };

    users.users.matrix-authentication-service = {
      group = "matrix-authentication-service";
      home = "/var/lib/matrix-authentication-service";
      isSystemUser = true;
    };

    users.groups.matrix-authentication-service = { };

    ${namespace} = {
      ports = {
        mas = 8081;
        mas-internal = 8082;
      };
      services.monitoring.agent.targets.mas.port = ports.mas-internal;

      system.impermanence.directories = [
        "/var/lib/private/matrix-authentication-service"
      ];
    };

    services.caddy.virtualHosts."${authHost}".extraConfig = ''
      reverse_proxy 127.0.0.1:${toString ports.mas}
    '';

    services.matrix-authentication-service = {
      enable = true;

      credentials = {
        shared-secret = vars.matrix-mas-shared-secret.files.secret.path;
        encryption = vars.matrix-mas-encryption.files.encryption.path;
        rsa = vars.matrix-mas-rsa-key.files.rsa.path;
        ec-prime256v1 = vars.matrix-mas-ec-key.files.ec-prime256v1.path;
      };

      settings = {
        http = {
          public_base = publicBase;
          issuer = publicBase;
          trusted_proxies = [
            "127.0.0.1/32"
            "::1/128"
          ];
          listeners = [
            {
              name = "web";
              binds = [
                {
                  host = "127.0.0.1";
                  port = ports.mas;
                }
              ];
              resources = [
                { name = "discovery"; }
                { name = "human"; }
                { name = "oauth"; }
                { name = "compat"; }
                { name = "graphql"; }
                { name = "assets"; }
              ];
            }
            {
              name = "internal";
              binds = [
                {
                  host = "127.0.0.1";
                  port = ports.mas-internal;
                }
              ];
              resources = [
                { name = "health"; }
                { name = "prometheus"; }
              ];
            }
          ];
        };

        database.uri = "postgresql:///matrix-authentication-service?host=/run/postgresql&user=matrix-authentication-service";

        matrix = {
          kind = "synapse";
          homeserver = baseDomain;
          endpoint = "http://127.0.0.1:${toString ports.synapse}";
          secret_file = credential "shared-secret";
        };

        secrets = {
          encryption_file = credential "encryption";
          keys = [
            {
              kid = "rsa-2026-04";
              key_file = credential "rsa";
            }
            {
              kid = "ec-prime256v1-2026-04";
              key_file = credential "ec-prime256v1";
            }
          ];
        };

        account = {
          password_registration_enabled = true;
          password_registration_email_required = false;
          registration_token_required = true;
          email_change_allowed = false;
          displayname_change_allowed = true;
          password_change_allowed = true;
          password_recovery_enabled = false;
          account_deactivation_allowed = true;
          login_with_email_allowed = false;
        };

        email.transport = "blackhole";

        branding.service_name = "Elxreno Matrix";

        telemetry.metrics.exporter = "prometheus";
      };
    };

    systemd.services.matrix-authentication-service = {
      after = [
        "network-online.target"
        "postgresql-databases-init.service"
      ];
      requires = [ "postgresql-databases-init.service" ];
      wants = [ "network-online.target" ];
    };
  };
}

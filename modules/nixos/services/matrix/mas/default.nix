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
    ;
  cfg = config.${namespace}.services.matrix.mas;
  synapseCfg = config.${namespace}.services.matrix.synapse;

  baseDomain = synapseCfg.serverName;
  authHost = "auth.${baseDomain}";
  publicBase = "https://${authHost}/";

  yamlFormat = pkgs.formats.yaml { };

  credentialsDir = "/run/credentials/matrix-authentication-service.service";

  staticSettings = {
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
          binds = [ { address = "127.0.0.1:8081"; } ];
          resources = [
            { name = "discovery"; }
            { name = "human"; }
            { name = "oauth"; }
            { name = "compat"; }
            { name = "assets"; }
          ];
        }
        {
          name = "internal";
          binds = [ { address = "127.0.0.1:8082"; } ];
          resources = [
            { name = "health"; }
            { name = "prometheus"; }
          ];
        }
      ];
    };

    database = {
      uri = "postgresql:///matrix-authentication-service?host=/run/postgresql";
    };

    matrix = {
      kind = "synapse";
      homeserver = baseDomain;
      endpoint = "http://127.0.0.1:8008";
      secret_file = "${credentialsDir}/shared-secret";
    };

    secrets = {
      encryption_file = "${credentialsDir}/encryption";
      keys = [
        {
          kid = "rsa-2026-04";
          key_file = "${credentialsDir}/rsa";
        }
        {
          kid = "ec-prime256v1-2026-04";
          key_file = "${credentialsDir}/ec-prime256v1";
        }
      ];
    };

    passwords = {
      enabled = true;
      schemes = [
        {
          version = 1;
          algorithm = "argon2id";
        }
      ];
      minimum_complexity = 3;
    };

    account = {
      password_registration_enabled = false;
      registration_token_required = true;
      email_change_allowed = false;
      displayname_change_allowed = true;
      password_change_allowed = true;
      password_recovery_enabled = false;
      account_deactivation_allowed = true;
      login_with_email_allowed = false;
    };

    email = {
      transport = "blackhole";
    };

    branding = {
      service_name = "Elxreno Matrix";
    };

    telemetry = {
      metrics = {
        exporter = "prometheus";
      };
    };
  };

  configFile = yamlFormat.generate "matrix-authentication-service.yaml" staticSettings;
in
{
  options.${namespace}.services.matrix.mas = {
    enable = mkEnableOption "Whether to manage Matrix Authentication Service.";
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = synapseCfg.enable;
        message = "${namespace}.services.matrix.mas requires matrix.synapse to be enabled.";
      }
    ];

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
      system.impermanence.directories = [
        "/var/lib/matrix-authentication-service"
      ];

      services.nginx.virtualHosts."${authHost}" = {
        locations."/" = {
          proxyPass = "http://127.0.0.1:8081";
        };
      };
    };

    systemd.services.matrix-authentication-service = {
      description = "Matrix Authentication Service";
      documentation = [ "https://element-hq.github.io/matrix-authentication-service/" ];
      wantedBy = [ "multi-user.target" ];
      after = [
        "network-online.target"
        "matrix-postgres-init.service"
      ];
      requires = [ "matrix-postgres-init.service" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        Type = "exec";
        User = "matrix-authentication-service";
        Group = "matrix-authentication-service";
        StateDirectory = "matrix-authentication-service";
        StateDirectoryMode = "0750";
        WorkingDirectory = "/var/lib/matrix-authentication-service";
        Restart = "on-failure";
        RestartSec = "5s";

        LoadCredential = [
          "shared-secret:${config.clan.core.vars.generators.matrix-mas-shared-secret.files.secret.path}"
          "encryption:${config.clan.core.vars.generators.matrix-mas-encryption.files.encryption.path}"
          "rsa:${config.clan.core.vars.generators.matrix-mas-rsa-key.files.rsa.path}"
          "ec-prime256v1:${config.clan.core.vars.generators.matrix-mas-ec-key.files.ec-prime256v1.path}"
        ];

        ExecStartPre = [
          "${pkgs.matrix-authentication-service}/bin/mas-cli config check --config=${configFile}"
          "${pkgs.matrix-authentication-service}/bin/mas-cli database migrate --config=${configFile}"
        ];
        ExecStart = "${pkgs.matrix-authentication-service}/bin/mas-cli server --config=${configFile}";

        CapabilityBoundingSet = [ "" ];
        LockPersonality = true;
        MemoryDenyWriteExecute = false;
        NoNewPrivileges = true;
        PrivateDevices = true;
        PrivateTmp = true;
        ProcSubset = "pid";
        ProtectClock = true;
        ProtectControlGroups = true;
        ProtectHome = true;
        ProtectHostname = true;
        ProtectKernelLogs = true;
        ProtectKernelModules = true;
        ProtectKernelTunables = true;
        ProtectProc = "invisible";
        ProtectSystem = "strict";
        RemoveIPC = true;
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
          "AF_UNIX"
        ];
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        SystemCallArchitectures = "native";
        SystemCallFilter = [
          "@system-service"
          "~@privileged"
          "~@resources"
        ];
        UMask = "0077";
      };
    };
  };
}

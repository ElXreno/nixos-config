{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    concatMapStringsSep
    escapeShellArg
    getExe
    mapAttrsToList
    mkEnableOption
    mkIf
    mkOption
    mkPackageOption
    types
    ;
  cfg = config.${namespace}.services.garage;
  ports = config.${namespace}.ports;
  vars = config.clan.core.vars.generators;

  secretEnvironment = [
    "GARAGE_RPC_SECRET_FILE=%d/rpc-secret"
    "GARAGE_ALLOW_WORLD_READABLE_SECRETS=true"
  ];

  bootstrap = pkgs.writeShellApplication {
    name = "garage-bootstrap";
    runtimeInputs = [
      cfg.package
      pkgs.gawk
    ];
    text = ''
      until garage status >/dev/null 2>&1; do
        sleep 1
      done

      if garage status | grep -q "NO ROLE ASSIGNED"; then
        node=$(garage node id -q | cut -d@ -f1)
        garage layout assign -z ${escapeShellArg cfg.zone} -c ${escapeShellArg cfg.capacity} "$node"
        garage layout apply --version 1
      fi

      ${concatMapStringsSep "\n" (bucket: ''
        if ! garage key info "$(< ${escapeShellArg bucket.accessKeyFile})" >/dev/null 2>&1; then
          garage key import --yes -n ${escapeShellArg bucket.name} \
            "$(< ${escapeShellArg bucket.accessKeyFile})" "$(< ${escapeShellArg bucket.secretKeyFile})" >/dev/null
        fi
        garage bucket info ${escapeShellArg bucket.name} >/dev/null 2>&1 ||
          garage bucket create ${escapeShellArg bucket.name}
        garage bucket allow --read --write --owner ${escapeShellArg bucket.name} \
          --key "$(< ${escapeShellArg bucket.accessKeyFile})" >/dev/null
      '') (mapAttrsToList (name: bucket: bucket // { inherit name; }) cfg.buckets)}
    '';
  };
in
{
  options.${namespace}.services.garage = {
    enable = mkEnableOption "a single-node Garage object store";
    package = mkPackageOption pkgs "garage_2" { };

    region = mkOption {
      type = types.str;
      default = "garage";
      description = "S3 region name clients must sign requests for.";
    };
    zone = mkOption {
      type = types.str;
      default = "dc1";
      description = "Zone of the only node in the cluster layout.";
    };
    capacity = mkOption {
      type = types.str;
      example = "500GB";
      description = "Capacity advertised for the node in the cluster layout.";
    };

    buckets = mkOption {
      default = { };
      description = "Buckets to create, each owned by the one access key given for it.";
      type = types.attrsOf (
        types.submodule {
          options = {
            accessKeyFile = mkOption {
              type = types.path;
              description = "File with the access key ID, `GK` followed by 24 hex digits.";
            };
            secretKeyFile = mkOption {
              type = types.path;
              description = "File with the secret access key, 64 hex digits.";
            };
          };
        }
      );
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.ports = {
      garage-s3 = 3900;
      garage-rpc = 3901;
    };

    clan.core.vars.generators.garage = {
      files.rpc-secret.secret = true;
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl rand -hex 32 | tr -d '\n' > "$out/rpc-secret"
      '';
    };

    services.garage = {
      enable = true;
      inherit (cfg) package;
      settings = {
        db_engine = "lmdb";
        replication_factor = 1;
        compression_level = "none";
        rpc_bind_addr = "127.0.0.1:${toString ports.garage-rpc}";
        rpc_public_addr = "127.0.0.1:${toString ports.garage-rpc}";
        s3_api = {
          s3_region = cfg.region;
          api_bind_addr = "[::]:${toString ports.garage-s3}";
        };
      };
    };

    systemd.services.garage.serviceConfig = {
      LoadCredential = "rpc-secret:${vars.garage.files.rpc-secret.path}";
      Environment = secretEnvironment;
    };

    systemd.services.garage-bootstrap = {
      description = "Garage layout, keys and buckets";
      wantedBy = [ "multi-user.target" ];
      after = [ "garage.service" ];
      requires = [ "garage.service" ];
      restartTriggers = [ config.environment.etc."garage.toml".source ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        TimeoutStartSec = 120;
        ExecStart = getExe bootstrap;
        LoadCredential = "rpc-secret:${vars.garage.files.rpc-secret.path}";
        Environment = secretEnvironment;
      };
    };
  };
}

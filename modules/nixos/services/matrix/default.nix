{
  config,
  options,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkMerge
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.matrix;
in
{
  options.${namespace}.services.matrix = {
    enable = mkEnableOption "the Matrix stack (Synapse and MAS)";
    retain = mkEnableOption "backing up its data while the service is off" // {
      default = options.${namespace}.services.matrix.enable.highestPrio < 1500;
      defaultText = "Whether this host sets `enable` at all, even to false.";
    };

    voip.enable = mkEnableOption "Matrix voice and video (coturn, LiveKit, Element Call)" // {
      default = true;
    };

    serverName = mkOption {
      type = types.str;
      default = "elxreno.com";
      description = "The Matrix `server_name`, the permanent suffix of every Matrix ID.";
    };

    wellKnownClientExtra = mkOption {
      type = types.attrs;
      default = { };
      description = "Extra attributes merged into the `/.well-known/matrix/client` JSON.";
    };
  };

  config = mkMerge [
    (mkIf (cfg.enable || cfg.retain) {
      ${namespace}.services.postgresql = {
        enable = true;
        databases = {
          matrix-synapse = {
            locale = "C";
            backup = true;
          };
          matrix-authentication-service = {
            locale = "C";
            backup = true;
          };
        };
      };

    })
    (mkIf cfg.enable {
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
    })
  ];
}

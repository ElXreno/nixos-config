{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.server;
in
{
  options.${namespace}.roles.server = {
    enable = mkEnableOption "Whether or not to enable server configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      services.scx = {
        enable = true;
        scheduler = "scx_p2dq";
        schedulerExtraArgs = [ "--keep-running" ];
      };

      system = {
        nix = {
          auto-optimise.enable = true;
          gc.enable = true;
        };

        documentation.enable = false;
      };
    };
  };
}

{
  config,
  osConfig,
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
    enable = mkEnableOption "server configuration" // {
      default = osConfig.${namespace}.roles.server.enable;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;
      common-packages.enable = true;
      programs.ssh.enable = true;
    };
  };
}

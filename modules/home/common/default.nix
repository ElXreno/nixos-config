{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.common;
in
{
  options.${namespace}.common = {
    enable = mkEnableOption "Whether or not to configure common stuff for everything." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    home.stateVersion = osConfig.system.stateVersion;
  };
}

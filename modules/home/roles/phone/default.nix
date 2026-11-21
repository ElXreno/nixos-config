{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.phone;
in
{
  options.${namespace}.roles.phone = {
    enable = mkEnableOption "phone configuration" // {
      default = osConfig.${namespace}.roles.phone.enable;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      programs.firefox.enable = true;
    };
  };
}

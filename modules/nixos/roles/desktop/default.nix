{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.desktop;
in
{
  options.${namespace}.roles.desktop = {
    enable = mkEnableOption "Whether or not to enable desktop configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      common-packages.enable = true;

      services = {
        pipewire.enable = true;
      };
    };
  };
}

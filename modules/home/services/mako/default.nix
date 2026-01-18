{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.mako;
in
{
  options.${namespace}.services.mako = {
    enable = mkEnableOption "Whether or not to manage mako.";
  };

  config = mkIf cfg.enable {
    services.mako = {
      enable = true;

      settings = {
        border-radius = 8;
        padding = "8,12";
        max-icon-size = 48;
        default-timeout = 7000;
        group-by = "app-name,summary";

        on-button-middle = "dismiss-all";
      };
    };
  };
}

{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.mosquitto;
in
{
  options.${namespace}.services.mosquitto = {
    enable = mkEnableOption "Whether or not to manage mosquitto.";
  };

  config = mkIf cfg.enable {
    services.mosquitto = {
      enable = true;
    };
  };
}

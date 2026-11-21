{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.tablet;
in
{
  options.${namespace}.roles.tablet = {
    enable = mkEnableOption "tablet configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.laptop.enable = true;
    };

    hardware.sensor.iio.enable = true;
  };
}

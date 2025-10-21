{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.syncthing;
in
{
  options.${namespace}.services.syncthing = {
    enable = mkEnableOption "Whether or not to manage syncthing.";
  };

  config = mkIf cfg.enable {
    services.syncthing.enable = true;
  };
}

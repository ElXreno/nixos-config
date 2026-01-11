{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.syncthing;
in
{
  options.${namespace}.services.syncthing = {
    enable = mkEnableOption "Whether or not to manage syncthing.";
    settings = mkOption {
      type = types.raw;
      default = { };
    };
  };

  config = mkIf cfg.enable {
    services.syncthing = {
      enable = true;

      overrideDevices = true;
      overrideFolders = true;

      inherit (cfg) settings;
    };
  };
}

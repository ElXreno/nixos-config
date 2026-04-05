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
  cfg = config.${namespace}.services.thermald;
in
{
  options.${namespace}.services.thermald = {
    enable = mkEnableOption "Whether to manage thermald.";
    configFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Custom thermald configuration file.";
    };
  };

  config = mkIf cfg.enable {
    services.thermald = {
      enable = true;
      inherit (cfg) configFile;
    };
  };
}

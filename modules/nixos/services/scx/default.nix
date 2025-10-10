{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.scx;
in
{
  options.${namespace}.services.scx = {
    enable = mkEnableOption "Whether or not to manage scx.";
    scheduler = mkOption {
      type = with types; str;
      default = "scx_lavd";
    };
    schedulerExtraArgs = mkOption {
      type = with types; listOf str;
      default = [ "--autopower" ];
    };
  };

  config = mkIf cfg.enable {
    services.scx = {
      enable = true;
      scheduler = cfg.scheduler;
      extraArgs = cfg.schedulerExtraArgs;
    };
  };
}

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
  cfg = config.${namespace}.services.scx;
in
{
  options.${namespace}.services.scx = {
    enable = mkEnableOption "Whether or not to manage scx.";
    scheduler = mkOption {
      type = with types; str;
      default = "scx_bpfland";
    };
    schedulerExtraArgs = mkOption {
      type = with types; listOf str;
      default = [ ];
    };
  };

  config = mkIf cfg.enable {
    services.scx = {
      enable = true;
      inherit (cfg) scheduler;
      extraArgs = cfg.schedulerExtraArgs;
    };
  };
}

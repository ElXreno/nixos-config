{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.irqbalance;
in
{
  options.${namespace}.services.irqbalance = {
    enable = mkEnableOption "Whether or not to manage irqbalance.";
  };

  config = mkIf cfg.enable {
    services.irqbalance.enable = true;
  };
}

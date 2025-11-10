{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.asusd;
in
{
  options.${namespace}.services.asusd = {
    enable = mkEnableOption "Whether or not to manage asusd.";
  };

  config = mkIf cfg.enable {
    services.asusd = {
      enable = true;

      asusdConfig.source = ./asusd.ron;
      fanCurvesConfig.source = ./fan_curves.ron;
      auraConfigs.tuf.source = ./aura_tuf.ron;
    };

    systemd.services.asusd = {
      restartTriggers = with config.services.asusd; [
        asusdConfig.source
        fanCurvesConfig.source
        auraConfigs.tuf.source
      ];
    };
  };
}

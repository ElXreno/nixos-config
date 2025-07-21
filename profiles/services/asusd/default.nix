{...}: {
  services.asusd = {
    enable = true;
    enableUserService = true;

    asusdConfig.source = ./asusd.ron;
    fanCurvesConfig.source = ./fan_curves.ron;
    auraConfigs.tuf.source = ./aura_tuf.ron;
  };
}
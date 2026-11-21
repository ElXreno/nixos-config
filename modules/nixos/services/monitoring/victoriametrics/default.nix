{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.monitoring.victoriametrics;
in
{
  options.${namespace}.services.monitoring.victoriametrics = {
    enable = mkEnableOption "managing victoriametrics";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services.monitoring.agent.targets.victoriametrics.port = config.${namespace}.ports.victoriametrics;
      system.impermanence.directories = [
        {
          directory = "/var/lib/private/victoriametrics";
          user = "nobody";
          group = "nogroup";
          mode = "0700";
        }
      ];
    };

    services.victoriametrics = {
      enable = true;
      listenAddress = ":${toString config.${namespace}.ports.victoriametrics}";
      retentionPeriod = "2y";
    };
  };
}

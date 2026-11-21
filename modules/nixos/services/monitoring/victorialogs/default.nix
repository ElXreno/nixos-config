{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.monitoring.victorialogs;
in
{
  options.${namespace}.services.monitoring.victorialogs = {
    enable = mkEnableOption "managing victorialogs";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services.monitoring.agent.targets.victorialogs.port = config.${namespace}.ports.victorialogs;
      system.impermanence.directories = [
        {
          directory = "/var/lib/private/victorialogs";
          user = "nobody";
          group = "nogroup";
          mode = "0700";
        }
      ];
    };

    services.victorialogs = {
      enable = true;
      listenAddress = ":${toString config.${namespace}.ports.victorialogs}";
      extraOptions = [
        "-retentionPeriod=1y"
      ];
    };
  };
}

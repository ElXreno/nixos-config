{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.monitoring;
in
{
  options.${namespace}.services.monitoring = {
    enable = mkEnableOption "Whether to enable monitoring stack.";
  };

  config = mkIf cfg.enable {
    ${namespace}.services.monitoring = {
      grafana.enable = true;
      prometheus.exporters.enable = true;
      victorialogs.enable = true;
      victoriametrics.enable = true;
    };
  };
}

{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.psd;
in
{
  options.${namespace}.services.psd = {
    enable = mkEnableOption "Whether or not to manage profile-sync-daemon.";
  };

  config = mkIf cfg.enable {
    services.psd.enable = true;
  };
}

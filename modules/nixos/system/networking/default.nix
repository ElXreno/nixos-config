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
    ;
  cfg = config.${namespace}.system.networking;
in
{
  options.${namespace}.system.networking = {
    enable = mkEnableOption "Whether or not to manage networking." // {
      default = true;
    };
    networkmanager.enable = mkEnableOption "Whether to use NetworkManager." // {
      default = !config.${namespace}.roles.server.enable;
    };
  };

  config = mkIf cfg.enable {
    networking = {
      networkmanager = mkIf cfg.networkmanager.enable {
        enable = true;
        wifi.powersave = false;
      };
      useDHCP = false;
      useNetworkd = lib.mkDefault false;

      firewall = {
        pingLimit = "--limit 1/minute --limit-burst 5";
      };
    };

    systemd.services.NetworkManager-wait-online.enable = false;
  };
}

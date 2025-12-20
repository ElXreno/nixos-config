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
    optionals
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
    ${namespace}.system.impermanence = {
      directories = optionals cfg.networkmanager.enable [
        "/etc/NetworkManager/system-connections"
      ];
    };

    # for some reason *this* is what makes networkmanager not get screwed completely instead of the impermanence module
    systemd.tmpfiles.rules = optionals cfg.networkmanager.enable (
      with config.${namespace}.system.impermanence;
      [
        "L /var/lib/NetworkManager/secret_key - - - - ${defaultPersistentPath}/var/lib/NetworkManager/secret_key"
        "L /var/lib/NetworkManager/seen-bssids - - - - ${defaultPersistentPath}/var/lib/NetworkManager/seen-bssids"
        "L /var/lib/NetworkManager/timestamps - - - - ${defaultPersistentPath}/var/lib/NetworkManager/timestamps"
      ]
    );

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

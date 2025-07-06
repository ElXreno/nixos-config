{ config, lib, ... }:
{
  networking = {
    hostName = config.device;
    networkmanager = {
      enable = lib.mkIf (!config.deviceSpecific.isServer) true;
      wifi = {
        powersave = false;
      };
    };
    useDHCP = lib.mkIf (!config.deviceSpecific.isServer) (lib.mkDefault false);
    useNetworkd = lib.mkDefault false;
  };
}

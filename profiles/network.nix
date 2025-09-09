{ config, lib, ... }:
{
  networking = {
    hostName = config.device;
    networkmanager = {
      enable = lib.mkIf (!config.deviceSpecific.isServer) true;
      wifi = {
        backend = lib.mkIf (config.device == "KURWA") "iwd";
        powersave = false;
      };
    };
    useDHCP = false;
    useNetworkd = lib.mkOverride 10 false;
  };
}

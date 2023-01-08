{ config, lib, ... }:
{
  networking = {
    hostName = config.device;
    networkmanager = {
      enable = lib.mkIf (!config.deviceSpecific.isServer || config.device == "Noxer-Server") true;
      wifi = {
        powersave = false;
      };
    };
    useDHCP = lib.mkIf (!config.deviceSpecific.isServer) false;
  };
}

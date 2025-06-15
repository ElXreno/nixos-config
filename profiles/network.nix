{ config, lib, ... }:
{
  networking = {
    hostName = config.device.hostname;
    networkmanager = {
      enable = lib.mkIf (!config.device.isServer) true;
      wifi = {
        powersave = false;
      };
    };
    useDHCP = lib.mkIf (!config.device.isServer) (lib.mkDefault false);
  };
}

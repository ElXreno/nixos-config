{ config, lib, ... }:
{
  networking = {
    hostName = config.device;
    networkmanager = {
      enable = lib.mkIf (!config.deviceSpecific.isServer || config.device == "Noxer-Server") true;
      wifi = {
        backend = "iwd";
        powersave = false;
      };
    };
    useDHCP = lib.mkIf (!config.deviceSpecific.isServer) false;
    # fix iwd race by disabling iface management
    wireless.iwd.settings = {
      General = {
        UseDefaultInterface = true;
        AddressRandomization = "network";
        ManagementFrameProtection = 1;
        ControlPortOverNL80211 = true;
        DisableANQP = true;
      };
      Network = {
        EnableIPv6 = true;
      };
    };
  };
}

{ config, lib, ... }:
{
  networking = {
    hostName = config.device;
    networkmanager = {
      enable = lib.mkIf (!config.deviceSpecific.isServer) true;
      wifi = {
        # Disabled because I'm fucking tired from that buggy shEt
        #   backend = "iwd";
        macAddress = "random";
        powersave = false;
      };
    };
    useDHCP = lib.mkIf (!config.deviceSpecific.isServer) false;
    # fix iwd race by disabling iface management
    # wireless.iwd.settings = {
    #   General = {
    #     UseDefaultInterface = true;
    #     AddressRandomization = "network";
    #     ManagementFrameProtection = 1;
    #     ControlPortOverNL80211 = true;
    #     DisableANQP = true;
    #   };
    #   Settings = {
    #     AlwaysRandomizeAddress = true;
    #   };
    #   Network = {
    #     EnableIPv6 = true;
    #   };
    # };
  };

  services.avahi = lib.mkIf (!config.deviceSpecific.isServer) {
    enable = true;
    # publish = {
    #   enable = true;
    #   addresses = true;
    #   workstation = true;
    # };
  };
}

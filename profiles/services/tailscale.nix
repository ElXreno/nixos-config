{ config, lib, ... }: {
  services.tailscale = {
    enable = true;
    openFirewall = true;
    useRoutingFeatures = lib.mkIf config.deviceSpecific.isServer "server";
  };
}

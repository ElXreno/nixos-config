{ config, lib, ... }: {
  services.tailscale = {
    enable = true;
    openFirewall = true;
    useRoutingFeatures = lib.mkIf config.deviceSpecific.isServer "server";
    extraSetFlags =
      lib.mkIf config.deviceSpecific.isServer [ "--advertise-exit-node" ];
    permitCertUid =
      lib.mkIf config.services.caddy.enable config.services.caddy.user;
  };
}

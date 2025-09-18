{ config, lib, ... }:
let
  ts = config.services.tailscale;
in
{
  services.tailscale = {
    enable = true;
    openFirewall = true;
    useRoutingFeatures = "both";
    extraSetFlags = lib.optionals config.deviceSpecific.isServer [ "--advertise-exit-node" ];
    extraUpFlags = [ "--accept-dns=false" ]; # TODO: Use authKeyFile to get this working evverywhere
    permitCertUid = lib.mkIf config.services.caddy.enable config.services.caddy.user;
  };

  networking = {
    resolvconf.extraConfig = ''
      prepend_nameservers=100.100.100.100
      search_domains=angora-ide.ts.net
    '';

    networkmanager.unmanaged = with ts; [ interfaceName ];
    firewall.trustedInterfaces = with ts; [ interfaceName ];
  };

  systemd.services.tailscaled = {
    before = [ "network.target" ];
    after = [
      "dnscrypt-proxy.service"
    ];
  };
}

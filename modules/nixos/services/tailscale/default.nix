{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.tailscale;
in
{
  options.${namespace}.services.tailscale = {
    enable = mkEnableOption "Whether or not to manage tailscale.";
    isServer = mkEnableOption "Whether or not to configure tailscale for server.";
  };

  config = mkIf cfg.enable {
    services.tailscale = {
      enable = true;
      openFirewall = true;
      useRoutingFeatures = "both";
      extraSetFlags = lib.optionals cfg.isServer [ "--advertise-exit-node" ];
      extraUpFlags = [ "--accept-dns=false" ]; # TODO: Use authKeyFile to get this working evverywhere
      permitCertUid = lib.mkIf config.services.caddy.enable config.services.caddy.user;
    };

    networking =
      let
        ts = config.services.tailscale;
      in
      {
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
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption optionals;
  cfg = config.${namespace}.services.tailscale;
in
{
  options.${namespace}.services.tailscale = {
    enable = mkEnableOption "Whether or not to manage tailscale.";
    isServer = mkEnableOption "Whether or not to configure tailscale for server.";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = [
      "/var/lib/tailscale"
    ];

    sops.secrets.tailscale-auth-key = { };

    services.tailscale = {
      enable = true;
      authKeyFile = config.sops.secrets.tailscale-auth-key.path;
      openFirewall = true;
      useRoutingFeatures = "both";
      extraSetFlags = optionals cfg.isServer [ "--advertise-exit-node" ];
      extraUpFlags = [ "--accept-dns=false" ];
      permitCertUid = with config.services.caddy; mkIf enable user;
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

        firewall.trustedInterfaces = with ts; [ interfaceName ];
        networkmanager = {
          unmanaged = with ts; [ interfaceName ];
          dispatcherScripts = [
            # https://tailscale.com/docs/reference/best-practices/performance#ethtool-configuration
            {
              source = pkgs.writeShellScript "configure-udp-gro-forwarding-hook" ''
                INTERFACE=$1
                ACTION=$2
                LOG_IDENTIFIER=configure-udp-gro-forwarding-hook

                logger -t $LOG_IDENTIFIER "Event: $ACTION on $INTERFACE"

                if [ "$ACTION" = "up" ] || [ "$ACTION" = "dhcp4-change" ]; then
                  logger -t $LOG_IDENTIFIER "Attempting to enable UDP GRO on $INTERFACE"
                  ${lib.getExe pkgs.ethtool} -K "$INTERFACE" rx-udp-gro-forwarding on rx-gro-list off >/dev/null 2>&1 || true
                fi
              '';
            }
          ];
        };
      };

    systemd.services = {
      tailscaled = {
        before = [ "network.target" ];
        after = [
          "dnscrypt-proxy.service"
        ];
      };

      # don't wait for this stupid thing to be done executing
      # i.e. when no wifi, system doesn't hang 3 minutes for this crap
      tailscaled-autoconnect = {
        serviceConfig.Type = lib.mkForce "exec";
        path = [
          # Haxxx for `No status data could be sent: $NOTIFY_SOCKET was not set`
          (pkgs.writeShellScriptBin "systemd-notify" "true")
        ];
      };
    };
  };
}

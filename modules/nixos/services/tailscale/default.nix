{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption optional;
  cfg = config.${namespace}.services.tailscale;
in
{
  options.${namespace}.services.tailscale = {
    enable = mkEnableOption "Whether or not to manage tailscale.";
    isServer = mkEnableOption "Whether or not to configure tailscale for server." // {
      default = config.${namespace}.roles.server.enable;
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = [
      "/var/lib/tailscale"
    ];

    clan.core.vars.generators.tailscale = {
      prompts.authKey = {
        description = ''
          Provide a tailscale "auth key" to connect to the desired network.
          See <https://login.tailscale.com/admin/settings/keys>.
        '';
        type = "line";
      };

      files.authKey.secret = true;

      script = ''
        cat $prompts/authKey > $out/authKey
      '';
    };

    services.tailscale = {
      enable = true;
      authKeyFile = config.clan.core.vars.generators.tailscale.files.authKey.path;
      openFirewall = true;
      useRoutingFeatures = "both";
      extraUpFlags = [
        "--reset"
      ];
      extraSetFlags = [
        "--accept-dns"
      ]
      ++ (optional cfg.isServer "--advertise-exit-node")
      ++ (optional (!cfg.isServer) "--exit-node-allow-lan-access");
      permitCertUid = with config.services.caddy; mkIf enable user;
    };

    networking =
      let
        ts = config.services.tailscale;
      in
      {
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

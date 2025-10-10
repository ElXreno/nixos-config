{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.system.networking;
in
{
  options.${namespace}.system.networking = {
    enable = mkEnableOption "Whether or not to manage networking." // {
      default = true;
    };
    networkmanager.enable = mkEnableOption "Whether to use NetworkManager." // {
      default = !config.${namespace}.roles.server.enable;
    };
  };

  config = mkIf cfg.enable {
    networking = {
      networkmanager = mkIf cfg.networkmanager.enable {
        enable = true;
        wifi.powersave = false;
      };
      useDHCP = false;
      useNetworkd = lib.mkDefault false;
    };

    systemd.services.NetworkManager-wait-online.enable = false;

    environment.etc."NetworkManager/dispatcher.d/99-fq" = mkIf cfg.networkmanager.enable {
      source =
        let
          fqDispatcher = pkgs.writeShellScript "fq-dispatcher.sh" ''
            interface="$1"
            action="$2"

            if [ "$action" = "up" ] && [ "$interface" != "lo" ] && [ "$interface" != "tailscale0" ]; then
              ${pkgs.iproute2}/bin/tc qdisc replace dev "$interface" root fq

              echo "FQ applied to $interface"
            fi
          '';
        in
        fqDispatcher;
      mode = "0755";
    };
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    concatStringsSep
    mapAttrsToList
    mkDefault
    mkIf
    ;
  tunnel = config.${namespace}.system.networking.tunnels.airvpn;

  # enabled for DHT crawlers leaving thousands of short-lived flows behind
  conntrack = {
    tcp_timeout_syn_sent = 15;
    tcp_timeout_unacknowledged = 30;
    tcp_timeout_fin_wait = 10;
    tcp_timeout_close_wait = 10;
    tcp_timeout_last_ack = 10;
    tcp_timeout_time_wait = 10;
    udp_timeout = 5;
    udp_timeout_stream = 15;
  };

  # workaround for network-online.target reached before the uplink routes
  waitUplink = pkgs.writeShellScript "airvpn-wait-uplink" ''
    i=0
    while [ "$i" -lt 60 ]; do
      gw=$(${pkgs.iproute2}/bin/ip route show default | ${pkgs.gawk}/bin/awk '/default/ { print $3; exit }')
      if [ -n "$gw" ] && ${pkgs.iputils}/bin/ping -c1 -W1 "$gw" > /dev/null 2>&1; then
        exit 0
      fi
      ${pkgs.coreutils}/bin/sleep 2
      i=$((i + 1))
    done
  '';
in
{
  ${namespace}.system.networking.tunnels.airvpn = {
    mtu = mkDefault 1320;
    generator.description = "AirVPN wg-quick config.";
  };

  systemd.services.airvpn = mkIf tunnel.enable {
    startLimitIntervalSec = 0;
    serviceConfig.Restart = "on-failure";
    serviceConfig.RestartSec = 3;
    serviceConfig.ExecStartPre = [ waitUplink ];
    serviceConfig.ExecStartPost = [
      "${pkgs.iproute2}/bin/ip netns exec airvpn ${pkgs.procps}/bin/sysctl -wq ${
        concatStringsSep " " (
          mapAttrsToList (name: seconds: "net.netfilter.nf_conntrack_${name}=${toString seconds}") conntrack
        )
      }"
    ];
  };
}

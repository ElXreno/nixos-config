{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkMerge;
  inherit (config.${namespace}.system.tuning) profile;

  isServer = profile == "server";
  isLaptop = profile == "laptop";
  isXanmod = config.${namespace}.system.boot.kernel.flavour == "xanmod";
in
{
  boot.kernel.sysctl = mkMerge [
    {
      "net.core.default_qdisc" = "fq";
      "net.ipv4.tcp_congestion_control" = "bbr";

      "net.ipv4.tcp_keepalive_intvl" = 20;
      "net.ipv4.tcp_keepalive_probes" = 4;
      "net.ipv4.tcp_keepalive_time" = 80;

      "net.ipv4.tcp_mtu_probing" = 1;
      "net.ipv4.tcp_slow_start_after_idle" = 0;
      "net.ipv4.tcp_tw_reuse" = 1;

      "net.ipv4.udp_rmem_min" = 16384;
      "net.ipv4.udp_wmem_min" = 16384;

      "net.ipv4.conf.all.arp_ignore" = 1;
      "net.ipv4.conf.all.arp_announce" = 2;

      "net.core.rmem_max" = 134217728;
      "net.core.wmem_max" = 134217728;
      "net.ipv4.tcp_wmem" = "4096 65536 67108864";
    }
    (mkIf (!isServer) {
      "net.core.netdev_max_backlog" = 5000;
      "net.ipv4.tcp_notsent_lowat" = 16384;

      "net.ipv4.tcp_rmem" = "4096 131072 67108864";

      "net.core.rmem_default" = 4194304;
      "net.core.wmem_default" = 2097152;

      "net.core.optmem_max" = 65536;
    })
    (mkIf isServer {
      "net.core.rmem_default" = 16777216;
      "net.core.wmem_default" = 16777216;

      "net.ipv4.tcp_rmem" = "4096 87380 67108864";

      "net.core.netdev_max_backlog" = 250000;

      "net.ipv4.tcp_fastopen" = 3;

      "net.netfilter.nf_conntrack_max" = 524288;
      "net.netfilter.nf_conntrack_tcp_timeout_established" = 300;

      "net.ipv4.tcp_syncookies" = 1;
      "net.ipv4.tcp_synack_retries" = 3;
      "net.ipv4.tcp_syn_retries" = 3;
    })
    (mkIf isLaptop {
      "net.core.busy_poll" = 50;
      "net.core.busy_read" = 50;
    })
    (mkIf (isXanmod && !isServer) {
      "net.ipv4.tcp_collapse_max_bytes" = 1048576;
    })
  ];
}

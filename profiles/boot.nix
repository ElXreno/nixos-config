{ config, pkgs, lib, ... }:

let inherit (config.deviceSpecific) isDesktop isLaptop isServer;
in
{
  boot = {
    loader = {
      timeout = if isServer then 0 else 3;
    } // (
      if config.deviceSpecific.usesCustomBootloader then { }
      else if config.deviceSpecific.devInfo.legacy then {
        grub = {
          enable = true;
          version = 2;
          device = lib.mkDefault "/dev/sda";
        };
      } else {
        efi.canTouchEfiVariables = true;
        systemd-boot = {
          enable = true;
          configurationLimit = 3;
        };
      }
    );
    kernelPackages = lib.mkMerge [
      (lib.mkIf isLaptop pkgs.linuxPackages_xanmod_latest)
      (lib.mkIf isDesktop pkgs.linuxPackages_5_15)
      # Servers
      (lib.mkIf isServer pkgs.linuxPackages_latest)
    ];

    kernelParams = [
      "nohibernate"
    ] ++ lib.optionals (isDesktop || isLaptop || config.device == "Noxer-Server") [
      "i915.mitigations=off"
      "mitigations=off"
    ] ++ lib.optionals (isDesktop || isLaptop) [
      "preempt=full"
    ] ++ lib.optionals isDesktop [
      "systemd.gpt_auto=0"
    ];

    kernel.sysctl = lib.mkMerge [
      {
        # Network
        "net.core.default_qdisc" = "fq";
        "net.core.netdev_max_backlog" = "16384";
        "net.ipv4.tcp_congestion_control" = "bbr";
        "net.ipv4.tcp_ecn" = 1;
        "net.ipv4.tcp_keepalive_intvl" = 10;
        "net.ipv4.tcp_keepalive_probes" = 6;
        "net.ipv4.tcp_keepalive_time" = 30;
        "net.ipv4.tcp_low_latency" = 1;
        "net.ipv4.tcp_mtu_probing" = 1;
        "net.ipv4.tcp_notsent_lowat" = "16384";
        "net.ipv4.tcp_slow_start_after_idle" = 0;
        "net.ipv4.tcp_tw_reuse" = 1;

        # Memory
        "vm.oom_kill_allocating_task" = 1;
        "vm.swappiness" = 80;
      }
      (
        lib.mkIf (!isServer)
          {
            "kernel.sysrq" = 1;
          }
      )
      (
        lib.mkIf isDesktop
          {
            "vm.swappiness" = lib.mkForce 200;
          }
      )
      (
        lib.mkIf isLaptop
          {
            # Network
            "net.ipv4.ip_default_ttl" = 65;
            "net.ipv6.conf.all.hop_limit" = 65;
            "net.ipv6.conf.default.hop_limit" = 65;
            "net.ipv6.conf.lo.hop_limit" = 65;
            "net.ipv6.conf.wlp1s0.hop_limit" = 65;

            # Memory
            "vm.min_free_kbytes" = 157057;
            "vm.extfrag_threshold" = 0;
            "vm.vfs_cache_pressure" = 3000;
            "vm.page-cluster" = 0;
          }
      )
    ];
    supportedFilesystems = lib.mkIf (!isServer) [ "ntfs" ];
    tmpOnTmpfs = config.device != "Nixis-Server";
    tmpOnTmpfsSize = lib.mkIf config.zramSwap.enable "180%";
  };

  systemd.services.fq-as-default = lib.mkIf isLaptop {
    script = ''
      for interface in $(ls /sys/class/net/); do
        if [ "$interface" != "lo" ]; then
          if ${pkgs.iproute2}/bin/tc qdisc show dev $interface | grep -q "qdisc noqueue"; then
            ${pkgs.iproute2}/bin/tc qdisc add dev $interface handle 1: root fq
            echo "Added fq to $interface"
          else
            echo "$interface already has a qdisc"
          fi
        else
          echo "Skipping $interface"
        fi
      done
    '';
    serviceConfig.Type = "oneshot";
    wantedBy = [ "multi-user.target" ];
    after = [ "network.target" ];
  };
}

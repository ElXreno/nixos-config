{ config, pkgs, inputs, lib, ... }:
{
  boot = {
    loader = lib.mkIf (!config.deviceSpecific.isServer) {
      timeout = 3;
      grub = {
        enable = true;
        version = 2;
        device = "/dev/sda";
        useOSProber = true;
      };
    };
    kernelPackages = lib.mkMerge [
      (lib.mkIf config.deviceSpecific.isLaptop pkgs.linuxPackages_xanmod_latest)
      (lib.mkIf config.deviceSpecific.isDesktop pkgs.linuxPackages_5_15)
      # Servers
      (lib.mkIf config.deviceSpecific.isServer pkgs.linuxPackages_latest)
    ];
    kernelModules = [ "kvm-amd" "kvm-intel" ];
    kernelParams = lib.mkMerge [
      [
        "mitigations=off"
        "i915.mitigations=off"
        # "delayacct"
        "nohibernate"
      ]
      (
        lib.mkIf config.deviceSpecific.isDesktop
          [
            "systemd.gpt_auto=0"
          ]
      )
      (
        lib.mkIf (config.device == "Honor-MB-AMD-Laptop")
          [
            # Disable NVMe powersave because this is a piece of shit maked by WD
            # sometimes hangs
            # "nvme_core.default_ps_max_latency_us=0"
            "zfs.zfs_arc_min=1610612736"
            "zfs.zfs_arc_max=1610612736"
            "zfs.zfs_arc_sys_free=1073741824"
            "zfs.arc_shrink_shift=5"
            "zfs.metaslab_lba_weighting_enabled=0"
          ]
      )
    ];
    kernel.sysctl = lib.mkMerge [
      {
        "net.core.default_qdisc" = "fq";
        "net.core.netdev_max_backlog" = "16384";
        "net.ipv4.tcp_congestion_control" = "bbr";
        "net.ipv4.tcp_fastopen" = 3;
        "net.ipv4.tcp_keepalive_intvl" = 10;
        "net.ipv4.tcp_keepalive_probes" = 6;
        "net.ipv4.tcp_keepalive_time" = 60;
        "net.ipv4.tcp_mtu_probing" = 1;
        "net.ipv4.tcp_notsent_lowat" = "16384";
        "net.ipv4.tcp_slow_start_after_idle" = 0;
        "vm.oom_kill_allocating_task" = 1;
        "vm.overcommit_memory" = 2;
        "vm.overcommit_ratio" = 400;
        "vm.swappiness" = 80;
      }
      (
        lib.mkIf (!config.deviceSpecific.isServer)
          {
            "kernel.sysrq" = 1;
          }
      )
      (
        lib.mkIf config.deviceSpecific.isDesktop
          {
            "vm.swappiness" = lib.mkForce 200;
          }
      )
      (
        lib.mkIf config.deviceSpecific.isLaptop
          {
            "net.ipv4.ip_default_ttl" = 65;
            "net.ipv6.conf.all.hop_limit" = 65;
            "net.ipv6.conf.default.hop_limit" = 65;
            "net.ipv6.conf.lo.hop_limit" = 65;
            "net.ipv6.conf.wlp1s0.hop_limit" = 65;

            "net.ipv4.tcp_congestion_control" = lib.mkForce "bbr2";

            "vm.page-cluster" = 6;
          }
      )
    ];
    supportedFilesystems = lib.mkIf (!config.deviceSpecific.isServer) [ "ntfs" ];
    tmpOnTmpfs = config.device != "Honor-MB-AMD-Laptop";
  };

  systemd.services.fq-as-default = lib.mkIf config.deviceSpecific.isLaptop {
    script = ''
      export FILEPATH="/tmp/fq_pie-activated"
      if [ ! -f $FILEPATH ]; then
        ${pkgs.iproute2}/bin/tc qdisc add dev wlp1s0 handle 1: root fq
        touch $FILEPATH
      fi
    '';
    serviceConfig.Type = "oneshot";
    wantedBy = [ "multi-user.target" ];
  };
}

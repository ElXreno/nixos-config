{ config, pkgs, lib, ... }:

let inherit (config.deviceSpecific) isDesktop isLaptop isServer;
in {
  boot = {
    loader = {
      timeout = if isServer then 0 else 3;
    } // (if config.deviceSpecific.usesCustomBootloader then
      { }
    else if config.deviceSpecific.devInfo.legacy then {
      grub = {
        enable = true;
        device = lib.mkDefault "/dev/sda";
      };
    } else {
      efi.canTouchEfiVariables = true;
      systemd-boot = {
        enable = true;
        configurationLimit = 3;
      };
    });
    kernelPackages = lib.mkMerge [
      # (lib.mkIf isLaptop pkgs.linuxPackages_xanmod_latest)
      (lib.mkIf isLaptop pkgs.linuxPackages_6_14)
      (lib.mkIf isDesktop pkgs.linuxPackages_5_15)
      # Servers
      (lib.mkIf isServer pkgs.linuxPackages_latest)
    ];

    kernelParams = [ "nohibernate" ] ++ lib.optionals (isDesktop || isLaptop) [
      "i915.mitigations=off"
      # slowdown for INFINITY is minimal
      # https://browser.geekbench.com/v6/cpu/compare/1757356?baseline=1757440
      "mitigations=off"
    ] ++ lib.optionals (isDesktop || isLaptop) [ "preempt=full" ]
      ++ lib.optionals isDesktop [ "systemd.gpt_auto=0" ]
      ++ lib.optionals (config.device == "INFINITY") [
        # Disabled due 4k external monitor
        # "amdgpu.gttsize=1536"
        # TSC found unstable after boot, most likely due to broken BIOS. Use 'tsc=unstable'.
        # "tsc=unstable"
        "clocksource=tsc" # https://www.reddit.com/r/linuxquestions/comments/ts1hgw/comment/i2p1i90/
        "tsc=reliable"

        # https://discourse.ubuntu.com/t/fine-tuning-the-ubuntu-24-04-kernel-for-low-latency-throughput-and-power-efficiency/44834
        "nohz_full=all"
        "rcutree.enable_rcu_lazy=0"
      ];

    kernel.sysctl = lib.mkMerge [
      {
        # Network
        "net.core.default_qdisc" = "cake";
        "net.ipv4.tcp_congestion_control" = "bbr";

        "net.core.netdev_max_backlog" = "16384";
        "net.ipv4.tcp_notsent_lowat" = "16384";

        "net.ipv4.tcp_fastopen" = 3;

        "net.ipv4.tcp_keepalive_intvl" = 10;
        "net.ipv4.tcp_keepalive_probes" = 6;
        "net.ipv4.tcp_keepalive_time" = 30;

        "net.ipv4.tcp_mtu_probing" = 1;
        "net.ipv4.tcp_slow_start_after_idle" = 0;
        "net.ipv4.tcp_tw_reuse" = 1;

        # Memory
        "vm.oom_kill_allocating_task" = 1;
        "vm.page-cluster" = 0;
        "vm.swappiness" = 80;

        "kernel.split_lock_mitigate" = 0;
      }
      (lib.mkIf (!isServer) {
        "kernel.sysrq" = 1;
        "vm.compaction_proactiveness" = 0;
      })
      (lib.mkIf isDesktop { "vm.swappiness" = lib.mkForce 200; })
      (lib.mkIf isLaptop {
        # Network
        # "net.ipv4.ip_default_ttl" = 65;

        # Memory
        "vm.min_free_kbytes" = 262144;
        "vm.extfrag_threshold" = 300;
        "vm.vfs_cache_pressure" = 3000;
      })
    ];
    supportedFilesystems = lib.mkIf (!isServer) [ "ntfs" ];

    tmp = {
      # useTmpfs = true;
      tmpfsSize = lib.mkIf config.zramSwap.enable "180%";
    };

    # Decrypt LUKS via TPM2 on INFINITY
    initrd = lib.mkIf (config.device == "INFINITY") {
      systemd.enable = true;
      availableKernelModules = [ "tpm_crb" ];
    };

    plymouth.enable = !isServer;
  };

  systemd = {
    tmpfiles.rules = [ "w /sys/kernel/mm/lru_gen/min_ttl_ms - - - - 1500" ];
    extraConfig = ''
      DefaultOOMPolicy=continue
    '';
  };

  services.udev.extraRules = ''
    ACTION=="add|change", SUBSYSTEM=="block", KERNEL=="sd[a-z]", ATTR{queue/rotational}=="1", RUN+="${pkgs.smartmontools}/bin/smartctl -s apm,off /dev/%k"
  '';
}

{
  config,
  pkgs,
  lib,
  ...
}:

let
  inherit (config.deviceSpecific) isDesktop isLaptop isServer;
in
{
  boot = {
    loader = {
      timeout = if isServer then 0 else 3;
    }
    // (
      if config.deviceSpecific.usesCustomBootloader then
        { }
      else if config.deviceSpecific.devInfo.legacy then
        {
          grub = {
            enable = true;
            device = lib.mkDefault "/dev/sda";
          };
        }
      else
        {
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
      (lib.mkIf isServer pkgs.linuxPackages_latest)
    ];

    kernelParams = [
      "nohibernate"
    ]
    ++ lib.optionals (isDesktop || isLaptop) [
      "preempt=full"
      "nohz_full=all"
    ]
    ++ lib.optionals isDesktop [
      "systemd.gpt_auto=0"
      "mitigations=off"
    ]
    ++ lib.optionals isLaptop [
      "amdgpu.securedisplay=0"
      "threadirqs"
    ];

    kernel.sysctl = lib.mkMerge [
      {
        # Network
        "net.core.default_qdisc" = "fq";
        "net.ipv4.tcp_congestion_control" = "bbr";

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
        "net.core.netdev_max_backlog" = "2000";
        "net.ipv4.tcp_notsent_lowat" = "32768";

        "net.ipv4.tcp_rmem" = "4096 262144 8388608";
        "net.ipv4.tcp_wmem" = "4096 262144 8388608";
        "net.ipv4.tcp_adv_win_scale" = -2;
        "net.ipv4.tcp_collapse_max_bytes" = 1048576;

        # Memory
        "vm.min_free_kbytes" = 262144;
        "vm.extfrag_threshold" = 300;
        "vm.vfs_cache_pressure" = 3000;
      })
      (lib.mkIf (with config.services.dnscrypt-proxy2; enable && settings.http3) {
        "net.core.rmem_max" = 7500000;
        "net.core.wmem_max" = 7500000;
      })
    ];
    supportedFilesystems = lib.mkIf (!isServer) [ "ntfs" ];

    tmp = {
      # useTmpfs = true;
      tmpfsSize = lib.mkIf config.zramSwap.enable "180%";
    };

    # Decrypt LUKS via TPM2 on INFINITY
    initrd = {
      compressor = "zstd";
      compressorArgs = [
        "-19"
        "-T0"
      ];
    };

    plymouth.enable = !isServer;
  };

  environment.etc."NetworkManager/dispatcher.d/99-fq" =
    lib.mkIf config.networking.networkmanager.enable
      {
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

  systemd = {
    tmpfiles.rules = [ "w /sys/kernel/mm/lru_gen/min_ttl_ms - - - - 1500" ];
    settings.Manager = {
      DefaultOOMPolicy = "continue";
    };
  };

  services.udev.extraRules = ''
    ACTION=="add|change", SUBSYSTEM=="block", KERNEL=="sd[a-z]", ATTR{queue/rotational}=="1", RUN+="${pkgs.smartmontools}/bin/smartctl -s apm,off /dev/%k"
  '';
}

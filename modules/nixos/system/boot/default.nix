{
  config,
  namespace,
  virtual,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption mkMerge;
  cfg = config.${namespace}.system.boot;
  inherit (config.${namespace}) roles;

  isServer = roles.server.enable;
  isDesktop = roles.desktop.enable;
  isLaptop = roles.laptop.enable;
in
{
  options.${namespace}.system.boot = {
    enable = mkEnableOption "Whether or not to manage boot stuff.";
    setupBootloader = mkEnableOption "Whether to setup bootloader settings." // {
      default = true;
    };
    uefi.enable = mkEnableOption "Whether to setup boot for modern UEFI.";
    legacy = {
      enable = mkEnableOption "Whether to setup boot for legacy BIOS.";
      setupDevice = mkEnableOption "Whether to setup default device." // {
        default = true;
      };
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.uefi.enable || cfg.legacy.enable;
        message = "Please enable one of ${namespace}.system.boot.uefi.enable or ${namespace}.system.boot.legacy.enable.";
      }
      {
        assertion = !(cfg.uefi.enable && cfg.legacy.enable);
        message = "Disallowed use ${namespace}.system.boot.uefi.enable and ${namespace}.system.boot.legacy.enable simultaneously.";
      }
    ];

    boot = {
      loader = mkIf cfg.setupBootloader (mkMerge [
        { timeout = if isServer then 0 else 3; }

        (mkIf cfg.uefi.enable {
          efi.canTouchEfiVariables = true;
          systemd-boot = {
            enable = true;
            configurationLimit = 3;
          };
        })

        (mkIf cfg.legacy.enable {
          grub = {
            enable = true;
            device = lib.mkIf cfg.legacy.setupDevice (lib.mkDefault "/dev/sda");
          };
        })
      ]);

      kernelPackages = lib.mkMerge [
        (mkIf isLaptop pkgs.linuxPackages_xanmod_latest)
        (mkIf isDesktop pkgs.linuxPackages_5_15)
        (mkIf isServer pkgs.linuxPackages_latest)
      ];

      kernelParams = [
        "nohibernate"
      ]
      ++ lib.optionals (isDesktop || isLaptop) [
        "preempt=full"
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

          "net.ipv4.tcp_keepalive_intvl" = 20;
          "net.ipv4.tcp_keepalive_probes" = 4;
          "net.ipv4.tcp_keepalive_time" = 80;

          "net.ipv4.tcp_mtu_probing" = 1;
          "net.ipv4.tcp_slow_start_after_idle" = 0;
          "net.ipv4.tcp_tw_reuse" = 1;

          # Memory
          "vm.oom_kill_allocating_task" = 1;

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
          "net.ipv4.tcp_notsent_lowat" = "16384";

          "net.ipv4.tcp_rmem" = "8192 262144 33554432";
          "net.ipv4.tcp_wmem" = "4096 16384 16777216";
          "net.ipv4.tcp_adv_win_scale" = -2;
          "net.ipv4.tcp_collapse_max_bytes" = 6291456;

          # Memory
          "vm.min_free_kbytes" = 262144;
          "vm.extfrag_threshold" = 300;
          "vm.vfs_cache_pressure" = 3000;
        })
        (lib.mkIf (with config.services.dnscrypt-proxy; enable && settings.http3) {
          "net.core.rmem_max" = 33554432;
          "net.core.wmem_max" = 16777216;
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

      plymouth.enable = !isServer && !virtual;
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
  };
}

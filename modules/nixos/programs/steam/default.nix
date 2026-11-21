{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.steam;
in
{
  options.${namespace}.programs.steam = {
    enable = mkEnableOption "managing steam";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.networking.firewall.public.steam = {
      upstream = "nixos/modules/programs/steam.nix";
      tcp = [
        27015
        27036
        27037
        27040
      ];
      udp = [
        10400
        10401
        27015
        27036
      ];
      udpRanges = [
        {
          from = 27031;
          to = 27035;
        }
      ];
    };

    programs.steam = {
      enable = true;
      remotePlay.openFirewall = true;
      dedicatedServer.openFirewall = true;
      localNetworkGameTransfers.openFirewall = true;
      gamescopeSession.enable = true;
      extraPackages = [ pkgs.${namespace}.nms-vk-fix ];
    };

    environment.systemPackages = with pkgs; [
      protonup-qt
      vulkan-tools
    ];

    # last checked with https://steamdeck-packages.steamos.cloud/archlinux-mirror/jupiter-main/os/x86_64/steamos-customizations-jupiter-20251229.1-1-any.pkg.tar.zst
    boot.kernel.sysctl = {
      # 20-shed.conf
      "kernel.sched_cfs_bandwidth_slice_us" = 3000;
      # 20-net-timeout.conf
      # This is required due to some games being unable to reuse their TCP ports
      # if they're killed and restarted quickly - the default timeout is too large.
      "net.ipv4.tcp_fin_timeout" = 5;
      # 30-splitlock.conf
      # Prevents intentional slowdowns in case games experience split locks
      # This is valid for kernels v6.0+
      "kernel.split_lock_mitigate" = 0;
      # 30-vm.conf
      # USE MAX_INT - MAPCOUNT_ELF_CORE_MARGIN.
      # see comment in include/linux/mm.h in the kernel tree.
      "vm.max_map_count" = 2147483642;
    };

    boot.kernelModules = [ "ntsync" ];
    services.udev.packages = [
      (pkgs.writeTextFile {
        name = "ntsync-udev-rules";
        destination = "/lib/udev/rules.d/70-ntsync.rules";
        text = ''
          KERNEL=="ntsync", MODE="0644", TAG+="uaccess"
        '';
      })
    ];

    # Esync
    systemd.settings.Manager = {
      DefaultLimitNOFILE = 1048576;
    };
    security.pam.loginLimits = [
      {
        domain = config.${namespace}.primaryUser;
        type = "hard";
        item = "nofile";
        value = "1048576";
      }
    ];

    environment.sessionVariables = {
      PROTON_ENABLE_WAYLAND = "1";
      PROTON_NO_WM_DECORATION = "1";
      PROTON_USE_WOW64 = "1";
    };
  };
}

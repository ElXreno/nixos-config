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
    enable = mkEnableOption "Whether or not to manage steam.";
    xboxSupport = mkEnableOption "Whether to enable Xbox Controller support.";
  };

  config = mkIf cfg.enable {
    programs.steam = {
      enable = true;
      remotePlay.openFirewall = true;
      dedicatedServer.openFirewall = true;
      localNetworkGameTransfers.openFirewall = true;
      gamescopeSession.enable = true;
    };

    environment.systemPackages = with pkgs; [
      protonup-qt
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

    ${namespace} = {
      programs.gamemode.enable = true;
      programs.gamescope.enable = true;
      services.ananicy.enable = true;
      system.hardware.bluetooth.xboxSupport = cfg.xboxSupport;
    };
    hardware.xpadneo.enable = cfg.xboxSupport;

    boot.kernelModules = [ "ntsync" ];
    services.udev.extraRules = ''
      KERNEL=="ntsync", MODE="0644", TAG+="uaccess"
    '';

    # Esync
    systemd.settings.Manager = {
      DefaultLimitNOFILE = 524288;
    };
    security.pam.loginLimits = [
      {
        domain = "elxreno";
        type = "hard";
        item = "nofile";
        value = "524288";
      }
    ];

    environment.sessionVariables = {
      PROTON_ENABLE_WAYLAND = "1";
      PROTON_NO_WM_DECORATION = "1";
      PROTON_USE_NTSYNC = "1";
      PROTON_USE_WOW64 = "1";
    };
  };
}

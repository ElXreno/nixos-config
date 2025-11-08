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
    };

    environment.systemPackages = with pkgs; [
      protonup-qt
    ];

    boot.kernel.sysctl = {
      "kernel.split_lock_mitigate" = 0;
    };

    ${namespace} = {
      programs.gamemode.enable = true;
      system.hardware.bluetooth.xboxSupport = cfg.xboxSupport;
    };
    hardware.xpadneo.enable = cfg.xboxSupport;

    boot.kernelModules = [ "ntsync" ];
    services.udev.extraRules = ''
      KERNEL=="ntsync", MODE="0644", TAG+="uaccess"
    '';
    environment.sessionVariables = {
      PROTON_ENABLE_WAYLAND = "1";
      PROTON_NO_WM_DECORATION = "1";
      PROTON_USE_NTSYNC = "1";
      PROTON_USE_WOW64 = "1";
    };
  };
}

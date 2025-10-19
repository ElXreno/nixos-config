{
  config,
  namespace,
  lib,
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

    boot.kernel.sysctl = {
      "kernel.split_lock_mitigate" = 0;
    };

    ${namespace}.system.hardware.bluetooth.xboxSupport = cfg.xboxSupport;
    hardware.xpadneo.enable = cfg.xboxSupport;
  };
}

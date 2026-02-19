{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.monitoring.prometheus.exporters;

  supportedFilesystems = builtins.attrNames config.boot.supportedFilesystems;
  zfsEnabled = builtins.elem "zfs" supportedFilesystems;
  nvidiaEnabled = builtins.elem "nvidia" config.services.xserver.videoDrivers;
in
{
  options.${namespace}.services.monitoring.prometheus.exporters = {
    enable = mkEnableOption "Whether or not to manage prometheus exporters.";
    zfs.enable = mkEnableOption "Whether to enable zfs exporter." // {
      default = zfsEnabled;
    };
    nvidia.enable = mkEnableOption "Whether to enable nvidia exporter." // {
      default = nvidiaEnabled;
    };
  };

  config = mkIf cfg.enable {
    services.prometheus.exporters = {
      node = {
        enable = true;
        port = 9000;

        enabledCollectors = [
          "systemd"
          "processes"
          "ethtool"
          "softirqs"
          "tcpstat"
          "wifi"
          "interrupts"
        ];
      };

      zfs.enable = cfg.zfs.enable;

      smartctl.enable = true;

      # TODO: Restic

      nvidia-gpu.enable = cfg.nvidia.enable;
    };

    # Required for SMART exporter to access NVMe disks
    # https://github.com/NixOS/nixpkgs/issues/210041#issuecomment-1694704611
    services.udev.extraRules = ''
      SUBSYSTEM=="nvme", KERNEL=="nvme[0-9]*", GROUP="disk"
    '';
  };
}

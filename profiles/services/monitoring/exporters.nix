{ config, ... }:
let
  supportedFilesystems = builtins.attrNames config.boot.supportedFilesystems;
  zfsEnabled = builtins.elem "zfs" supportedFilesystems;
  nvidiaEnabled = builtins.elem "nvidia" config.services.xserver.videoDrivers;
in
{
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

    zfs.enable = zfsEnabled;

    smartctl.enable = true;

    # TODO: Restic

    nvidia-gpu = {
      enable = nvidiaEnabled;
    };
  };

  # Required for SMART to access NVMe disks
  # https://github.com/NixOS/nixpkgs/issues/210041#issuecomment-1694704611
  services.udev.extraRules = ''
    SUBSYSTEM=="nvme", KERNEL=="nvme[0-9]*", GROUP="disk"
  '';
}

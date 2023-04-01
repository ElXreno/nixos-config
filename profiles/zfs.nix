{ config, lib, ... }:

let inherit (config) device;
in
{
  # Enable zfs unstable for INFINITY until a new release
  # with 6.2 kernel support is published.
  boot.zfs.enableUnstable = config.device == "INFINITY";

  # This is not really needed, since zfs is already in the initrd
  # thanks to fileSystems."/" and fileSystems."/nix".
  boot.initrd.supportedFilesystems = lib.mkIf (device == "AMD-Desktop") [ "zfs" ];
  boot.supportedFilesystems = [ "zfs" ];

  boot.kernelParams = lib.optionals (device == "AMD-Desktop") [
    "zfs.zfs_arc_max=3758096384"
    "zfs.zfs_arc_min=1073741824"
    "zfs.zfs_prefetch_disable=1"
  ] ++ lib.optionals (device == "INFINITY") [
    "zfs.zfs_arc_min=536870912"
    "zfs.zfs_arc_max=1610612736"
    "zfs.zfs_arc_sys_free=1073741824"
    "zfs.arc_shrink_shift=5"
    "zfs.metaslab_lba_weighting_enabled=0"
  ];

  networking.hostId =
    if (device == "AMD-Desktop") then "2d73528c" else
    if (device == "INFINITY") then "20a7d5d8" else
    null;

  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="sd[a-z]*[0-9]*|mmcblk[0-9]*p[0-9]*|nvme[0-9]*n[0-9]*p[0-9]*", ENV{ID_FS_TYPE}=="zfs_member", ATTR{../queue/scheduler}="none"
  '';
}

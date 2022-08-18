{ config, inputs, pkgs, ... }:
{
  imports =
    [
      ./hardware-configuration.nix
      inputs.self.nixosRoles.desktop
      inputs.self.nixosProfiles.kde
    ];

  boot.loader.grub.copyKernels = true;

  networking.hostId = "2d73528c";

  boot.initrd.supportedFilesystems = [ "zfs" ];
  boot.supportedFilesystems = [ "zfs" ];
  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="sd[a-z]*[0-9]*|mmcblk[0-9]*p[0-9]*|nvme[0-9]*n[0-9]*p[0-9]*", ENV{ID_FS_TYPE}=="zfs_member", ATTR{../queue/scheduler}="none"
    ACTION=="add|change", KERNEL=="sd[a-z]", ATTRS{queue/rotational}=="1", RUN+="${pkgs.coreutils}/bin/echo 0 > /sys/block/sdb/queue/wbt_lat_usec"
  '';

  boot.kernelParams = [
    "zfs.zfs_arc_max=3758096384"
    "zfs.zfs_arc_min=1073741824"
    "zfs.zfs_prefetch_disable=1"
  ];

  i18n.defaultLocale = "ru_RU.UTF-8";

  services.xserver.videoDrivers = [ "nvidia" ];
  hardware.nvidia.package = config.boot.kernelPackages.nvidiaPackages.legacy_470;

  home-manager.users.alena = {
    home = {
      inherit (config.system) stateVersion;
      packages = with pkgs; [
        firefox-bin

        tdesktop

        # Office and language packs
        libreoffice
        hunspellDicts.ru-ru
      ];
    };
    services.syncthing.enable = true;
  };

  system.stateVersion = "22.05";
}

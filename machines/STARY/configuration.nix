{
  config,
  namespace,
  pkgs,
  ...
}:
{
  ${namespace} = {
    roles.server.enable = true;

    system = {
      boot = {
        legacy = {
          enable = true;
          setupDevice = false;
        };
        kernel.packages = pkgs.linuxPackages_6_12;
      };

      hardware = {
        cpu = {
          manufacturer = "amd";
          amd.zenpower.enable = false;
        };

        gpu.nvidia = {
          enable = true;
          package = config.boot.kernelPackages.nvidiaPackages.legacy_470;
          modesetting.enable = true;
        };

        hdd.hitachi.enable = true;
      };

      fs.zfs = {
        enable = true;
        hostId = "2d73528c";
      };

      zswap.createSwapfile = false;
    };

    services = {
      nixflix.enable = true;
      postgresql.enable = true;
    };
  };

  # mdadm monitoring not needed — boot mirror only
  boot.swraid.mdadmConf = "PROGRAM /run/current-system/sw/bin/true";

  # Super I/O sensor chip (Nuvoton NCT6776F) — not detected by facter
  boot.kernelModules = [ "nct6775" ];

  # ZFS tuning for HDD mirror + 12GB RAM
  boot.kernelParams = [
    "zfs.zfs_arc_max=6442450944"
    "zfs.zfs_txg_timeout=10"
    "zfs.zfs_dirty_data_max=536870912"
  ];

  # HDD write-back throttle tuning
  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="sd[a-z]", ATTRS{queue/rotational}=="1", RUN+="${pkgs.coreutils}/bin/echo 0 > /sys/block/%k/queue/wbt_lat_usec"
  '';
}

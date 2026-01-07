{
  pkgs,
  virtual,
  lib,
  ...
}:
{
  boot.initrd.systemd.services.initrd-rollback-root = lib.mkIf (!virtual) {
    after = [ "zfs-import-rpool.service" ];
    wantedBy = [ "initrd.target" ];
    before = [
      "sysroot.mount"
    ];
    path = [ pkgs.zfs ];
    description = "Rollback root fs";
    unitConfig.DefaultDependencies = "no";
    serviceConfig.Type = "oneshot";
    script = "zfs rollback -r rpool/root@blank";
  };

  disko.devices = {
    disk = {
      root = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-Samsung_SSD_9100_PRO_with_Heatsink_4TB_S7ZRNJ0Y301478E";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "4G";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            zfs = {
              size = "3T";
              content = {
                type = "zfs";
                pool = "rpool";
              };
            };
          };
        };
      };
    };
    zpool = {
      rpool = {
        options = {
          ashift = "12";
          autotrim = "on";
        };

        rootFsOptions = {
          compression = "zstd-3";
          acltype = "posixacl";
          xattr = "sa";
          dnodesize = "auto";
          normalization = "formD";
          atime = "off";
          canmount = "off";
          mountpoint = "none";

          encryption = "aes-256-gcm";
          keyformat = "passphrase";
          keylocation = "prompt";

          "com.sun:auto-snapshot" = "false";
        };

        datasets = {
          root = {
            type = "zfs_fs";
            mountpoint = "/";
            options = {
              mountpoint = "legacy";
              sync = "disabled";
            };
            postCreateHook = "zfs snapshot rpool/root@blank";
          };

          root-persist = {
            type = "zfs_fs";
            mountpoint = "/mnt/root-persist";
            options.mountpoint = "legacy";
          };

          nix = {
            type = "zfs_fs";
            mountpoint = "/nix";
            options = {
              mountpoint = "legacy";
              recordsize = "1M";
            };
          };

          home = {
            type = "zfs_fs";
            mountpoint = "/home";
            options.mountpoint = "legacy";
            postCreateHook = "zfs snapshot rpool/home@blank";
          };

          home-persist = {
            type = "zfs_fs";
            mountpoint = "/mnt/home-persist";
            options = {
              mountpoint = "legacy";
              "com.sun:auto-snapshot" = "true";
            };
          };

          reservation = {
            type = "zfs_fs";
            options = {
              refreservation = "10G";
              canmount = "off";
              mountpoint = "none";
            };
          };
        };
      };
    };
  };

  fileSystems = {
    "/mnt/root-persist".neededForBoot = true;
    "/mnt/home-persist".neededForBoot = true;
  };
}

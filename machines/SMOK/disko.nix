{ lib, ... }:
let
  mkDisk =
    {
      device,
      espMountpoint,
      espLabel ? null,
    }:
    {
      type = "disk";
      inherit device;
      content = {
        type = "gpt";
        partitions = {
          ESP = {
            size = "4G";
            type = "EF00";
            content = {
              type = "filesystem";
              format = "vfat";
              mountpoint = espMountpoint;
              mountOptions = [ "umask=0077" ];
            };
          }
          // lib.optionalAttrs (espLabel != null) { label = espLabel; };
          swap = {
            size = "16G";
            content = {
              type = "swap";
              priority = 10;
              randomEncryption = true;
            };
          };
          zfs = {
            size = "100%";
            content = {
              type = "zfs";
              pool = "rpool";
            };
          };
        };
      };
    };

in
{
  disko.devices = {
    disk = {
      nvme0 = mkDisk {
        device = "/dev/disk/by-id/nvme-Samsung_SSD_9100_PRO_with_Heatsink_4TB_S7ZRNJ0Y301506K";
        espMountpoint = "/boot";
        espLabel = "disk-root-ESP";
      };
      nvme1 = mkDisk {
        device = "/dev/disk/by-id/nvme-Samsung_SSD_9100_PRO_with_Heatsink_4TB_S7ZRNJ0Y301478E";
        espMountpoint = "/boot-fallback";
      };
      models = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-SAMSUNG_MZVLC2T0HBLD-00BT7_S85BNE0Y511033";
        content = {
          type = "gpt";
          partitions.models = {
            size = "100%";
            content = {
              type = "filesystem";
              format = "xfs";
              mountpoint = "/mnt/models";
              mountOptions = [ "noatime" ];
            };
          };
        };
      };
    };

    nodev."/" = {
      fsType = "tmpfs";
      mountOptions = [
        "size=4G"
        "mode=755"
      ];
    };

    zpool = {
      rpool = {
        type = "zpool";
        options = {
          ashift = "12";
          autotrim = "on";
          dedup_table_quota = "8G";
        };
        rootFsOptions = {
          canmount = "off";
          mountpoint = "none";
          encryption = "aes-256-gcm";
          keyformat = "passphrase";
          keylocation = "prompt";
          compression = "lz4";
          atime = "off";
          xattr = "sa";
          acltype = "posixacl";
          dnodesize = "auto";
        };

        datasets = {
          nix = {
            type = "zfs_fs";
            mountpoint = "/nix";
            options = {
              dedup = "blake3";
              direct = "disabled";
            };
          };
          root-persist = {
            type = "zfs_fs";
            mountpoint = "/mnt/root-persist";
          };
          "root-persist/postgres" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/postgresql";
            options = {
              recordsize = "16k";
              redundant_metadata = "most";
            };
          };
          "root-persist/libvirt" = {
            type = "zfs_fs";
            mountpoint = "/mnt/root-persist/var/lib/libvirt";
            options = {
              recordsize = "64k";
            };
          };
          home-persist = {
            type = "zfs_fs";
            mountpoint = "/home";
          };
          scratch = {
            type = "zfs_fs";
            mountpoint = "/mnt/scratch";
            options = {
              sync = "disabled";
              redundant_metadata = "some";
            };
          };
          datalake = {
            type = "zfs_fs";
            mountpoint = "/mnt/datalake";
            options = {
              recordsize = "1M";
              compression = "zstd-3";
              dedup = "blake3";
              direct = "disabled";
            };
          };
        };
      };
    };
  };
}

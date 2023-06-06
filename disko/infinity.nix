{ ... }: {
  disko.devices = {
    disk = {
      nvme = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-eui.002538d722a0adfe";
        content = {
          type = "table";
          format = "gpt";
          partitions = [
            {
              name = "ESP";
              start = "0";
              end = "512MiB";
              fs-type = "fat32";
              bootable = true;
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
              };
            }
            {
              name = "zfs";
              start = "512MiB";
              end = "75%";
              content = {
                type = "zfs";
                pool = "nvmepool";
              };
            }
          ];
        };
      };
    };
    zpool = {
      nvmepool = {
        type = "zpool";
        options = {
          ashift = "12";
          autotrim = "on";
        };
        rootFsOptions = {
          compression = "zstd";
          dedup = "on";
          encryption = "aes-256-gcm";
          keyformat = "passphrase";
          mountpoint = "none";
          relatime = "on";
          sync = "disabled";
          xattr = "sa";
        };
        postCreateHook = "zfs snapshot nvmepool@blank";

        datasets = {
          data = {
            type = "zfs_fs";
            options.mountpoint = "none";
          };
          "data/root" = {
            type = "zfs_fs";
            options.mountpoint = "legacy";
            mountpoint = "/";
          };
          "data/home" = {
            type = "zfs_fs";
            options = {
              mountpoint = "legacy";
              recordsize = "512K";
            };
            mountpoint = "/home";
          };
          "data/var" = {
            type = "zfs_fs";
            options = {
              mountpoint = "legacy";
              recordsize = "8K";
            };
            mountpoint = "/var";
          };
          nix = {
            type = "zfs_fs";
            options.mountpoint = "legacy";
            mountpoint = "/nix";
          };
          reservation = {
            type = "zfs_fs";
            options = {
              mountpoint = "none";
              refreservation = "2G";
            };
          };
        };
      };
    };
  };
}


{ ... }: {
  disko.devices = {
    disk = {
      x = {
        type = "disk";
        device = "/dev/disk/by-id/usb-JMicron_Generic_0123456789ABCDEF-0:0";
        content = {
          type = "table";
          format = "gpt";
          partitions = [
            {
              name = "zfs";
              start = "1MiB";
              end = "100%";
              content = {
                type = "zfs";
                pool = "externalhdd";
              };
            }
          ];
        };
      };
    };
    zpool = {
      externalhdd = {
        type = "zpool";
        rootFsOptions = {
          compression = "zstd";
          mountpoint = "none";
        };
        postCreateHook = "zfs snapshot externalhdd@blank";

        datasets = {
          tank = {
            type = "zfs_fs";
            options = {
              encryption = "aes-256-gcm";
              keyformat = "passphrase";
              mountpoint = "none";
              primarycache = "metadata";
              relatime = "on";
            };
          };
          "tank/unsafe" = {
            type = "zfs_fs";
            options = {
              dedup = "on";
              mountpoint = "/mnt/external-hdd/unsafe";
              sync = "disabled";
            };
          };
          "tank/safe" = {
            type = "zfs_fs";
            options = {
              copies = "2";
              mountpoint = "/mnt/external-hdd/safe";
            };
          };
          reservation = {
            type = "zfs_fs";
            options = {
              mountpoint = "none";
              refreservation = "1G";
            };
          };
        };
      };
    };
  };
}


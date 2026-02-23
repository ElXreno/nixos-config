{
  disko.devices = {
    disk = {
      hitachi = {
        type = "disk";
        device = "/dev/disk/by-id/wwn-0x5000cca216f2b95b";
        content = {
          type = "gpt";
          partitions = {
            bios = {
              size = "1M";
              type = "EF02";
            };
            boot = {
              size = "1G";
              content = {
                type = "mdraid";
                name = "boot";
              };
            };
            swap = {
              size = "4G";
              content = {
                type = "swap";
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
      wd = {
        type = "disk";
        device = "/dev/disk/by-id/wwn-0x50014ee2146b0822";
        content = {
          type = "gpt";
          partitions = {
            bios = {
              size = "1M";
              type = "EF02";
            };
            boot = {
              size = "1G";
              content = {
                type = "mdraid";
                name = "boot";
              };
            };
            swap = {
              size = "4G";
              content = {
                type = "swap";
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
    };

    mdadm = {
      boot = {
        type = "mdadm";
        level = 1;
        metadata = "1.0";
        content = {
          type = "filesystem";
          format = "ext4";
          mountpoint = "/boot";
          mountOptions = [ "noatime" ];
        };
      };
    };

    zpool = {
      rpool = {
        type = "zpool";
        mode = "mirror";
        options = {
          ashift = "12";
        };
        rootFsOptions = {
          compression = "zstd";
          atime = "off";
          xattr = "sa";
          acltype = "posixacl";
          dnodesize = "auto";
          mountpoint = "none";
        };

        datasets = {
          root = {
            type = "zfs_fs";
            mountpoint = "/";
            options = {
              mountpoint = "legacy";
              recordsize = "128K";
            };
          };
          nix = {
            type = "zfs_fs";
            mountpoint = "/nix";
            options = {
              mountpoint = "legacy";
              recordsize = "128K";
              sync = "disabled";
              redundant_metadata = "most";
            };
          };
          home = {
            type = "zfs_fs";
            mountpoint = "/home";
            options = {
              mountpoint = "legacy";
              recordsize = "128K";
            };
          };
          "safe/data" = {
            type = "zfs_fs";
            mountpoint = "/mnt/data";
            options = {
              mountpoint = "legacy";
              recordsize = "1M";
            };
          };
          "safe/backups" = {
            type = "zfs_fs";
            mountpoint = "/mnt/backups";
            options = {
              mountpoint = "legacy";
              recordsize = "1M";
            };
          };
          "nixflix/media" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/nixflix/media";
            options = {
              mountpoint = "legacy";
              recordsize = "1M";
              compression = "off";
              primarycache = "metadata";
              logbias = "throughput";
              redundant_metadata = "most";
            };
          };
          "nixflix/downloads" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/nixflix/downloads";
            options = {
              mountpoint = "legacy";
              recordsize = "1M";
              compression = "off";
              logbias = "throughput";
              redundant_metadata = "most";
            };
          };
          reserved = {
            type = "zfs_fs";
            options = {
              mountpoint = "none";
              refreservation = "50G";
            };
          };
        };
      };
    };
  };
}

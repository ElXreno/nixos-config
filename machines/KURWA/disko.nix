{
  disko.devices = {
    disk.nvme0 = {
      type = "disk";
      device = "/dev/disk/by-id/nvme-SAMSUNG_MZVL22T0HDLB-00BT7_S7CFNF0X506908";
      content = {
        type = "gpt";
        partitions = {
          ESP = {
            size = "4G";
            type = "EF00";
            label = "disk-root-ESP";
            content = {
              type = "filesystem";
              format = "vfat";
              mountpoint = "/boot";
              mountOptions = [ "umask=0077" ];
            };
          };
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

    nodev."/" = {
      fsType = "tmpfs";
      mountOptions = [
        "size=4G"
        "mode=755"
      ];
    };

    zpool.rpool = {
      type = "zpool";
      options = {
        ashift = "12";
        autotrim = "on";
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
        };
        root-persist = {
          type = "zfs_fs";
          mountpoint = "/mnt/root-persist";
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
      };
    };
  };
}

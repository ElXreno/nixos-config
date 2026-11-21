{
  config,
  ...
}:
{
  disko.devices = {
    disk = {
      vda = {
        type = "disk";
        device = "/dev/disk/by-path/pci-0000:00:10.0";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "1024M";
              type = "EF00";
              priority = 1;
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
    };

    zpool = {
      rpool = {
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
          keylocation = "file://${config.clan.core.vars.generators.zfs-passphrase.files.passphrase.path}";
          compression = "lz4";
          atime = "off";
          xattr = "sa";
          acltype = "posixacl";
          dnodesize = "auto";
        };
        postCreateHook = "zfs set keylocation=prompt rpool";

        datasets = {
          home-persist = {
            type = "zfs_fs";
            mountpoint = "/home";
          };
          nix = {
            type = "zfs_fs";
            mountpoint = "/nix";
          };
          root-persist = {
            type = "zfs_fs";
            mountpoint = "/";
          };
          "root-persist/garage" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/private/garage";
            options = {
              recordsize = "1M";
            };
          };
          "root-persist/clickhouse" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/clickhouse";
          };
          "root-persist/matrix" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/matrix-synapse";
            options = {
              recordsize = "1M";
            };
          };
          "root-persist/minecraft" = {
            type = "zfs_fs";
            mountpoint = "/srv/minecraft";
            options = {
              recordsize = "1M";
            };
          };
          "root-persist/ncps" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/ncps";
            options = {
              recordsize = "1M";
            };
          };
          "root-persist/ccache" = {
            type = "zfs_fs";
            mountpoint = "/mnt/scratch/ccache";
          };
          "root-persist/postgres" = {
            type = "zfs_fs";
            mountpoint = "/var/lib/postgresql";
            options = {
              recordsize = "16k";
              redundant_metadata = "most";
            };
          };
        };
      };
    };
  };
}

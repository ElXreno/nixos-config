let
  defaultMountOptions = [ "compress-force=zstd" ];
in
{
  fileSystems."/" = {
    device = "tmpfs";
    fsType = "tmpfs";
    options = [
      "size=4G"
      "mode=755"
    ];
  };

  disko.devices = {
    disk = {
      root = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-Samsung_SSD_980_1TB_S649NL1T779887H";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "2G";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            nix = {
              size = "200G";
              content = {
                type = "filesystem";
                format = "ext4";
                mountpoint = "/nix";
                mountOptions = [
                  "noatime"
                  "discard"
                ];
              };
            };
            data = {
              size = "100%";
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
                  "/home-persist" = {
                    mountpoint = "/home";
                    mountOptions = defaultMountOptions;
                  };
                  "/root-persist" = {
                    mountpoint = "/mnt/root-persist";
                    mountOptions = defaultMountOptions;
                  };
                };
              };
            };
          };
        };
      };
    };
  };

  fileSystems."/mnt/root-persist".neededForBoot = true;
}

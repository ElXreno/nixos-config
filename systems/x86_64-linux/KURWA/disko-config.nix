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
        device = "/dev/disk/by-id/nvme-Samsung_SSD_9100_PRO_with_Heatsink_4TB_S7ZRNJ0Y301506K";
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
            nix = {
              size = "400G";
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

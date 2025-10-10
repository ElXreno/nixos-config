_:
let
  defaultMountOptions = [ "compress-force=zstd" ];
in
{
  disko.devices = {
    disk = {
      nvme = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-Samsung_SSD_9100_PRO_with_Heatsink_4TB_S7ZRNJ0Y301506K";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "512M";
              type = "EF00";
              priority = 1;
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            root = {
              size = "100%";
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
                  "/root" = {
                    mountpoint = "/";
                    mountOptions = defaultMountOptions;
                  };
                  "/nix" = {
                    mountpoint = "/nix";
                    mountOptions = defaultMountOptions;
                  };
                  "/home" = {
                    mountpoint = "/home";
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
}

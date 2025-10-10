_:
let
  defaultMountOptions = [ "compress-force=zstd" ];
in
{
  disko.devices = {
    disk = {
      nvme = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-eui.e8238fa6bf530001001b448b448521c6";
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
              size = "150G";
              content = {
                type = "filesystem";
                format = "xfs";
                mountpoint = "/";
              };
            };
            home = {
              size = "100%";
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
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

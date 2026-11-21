{
  disko.devices =
    let
      mountOptions = [
        "noatime"
        "compress=zstd:1"
      ];
    in
    {
      disk.root = {
        type = "disk";
        device = "/dev/disk/by-id/mmc-MMC32G_0x0bbcd46f";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "1G";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            swap = {
              size = "4G";
              content.type = "swap";
            };
            root = {
              size = "100%";
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
                  "/root" = {
                    mountpoint = "/";
                    inherit mountOptions;
                  };
                  "/nix" = {
                    mountpoint = "/nix";
                    inherit mountOptions;
                  };
                  "/home" = {
                    mountpoint = "/home";
                    inherit mountOptions;
                  };
                };
              };
            };
          };
        };
      };
    };
}

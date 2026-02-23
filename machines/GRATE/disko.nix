# ---
# schema = "single-disk"
# [placeholders]
# mainDisk = "/dev/disk/by-id/ata-SK800-256GB_AA000000000000001697"
{
  disko.devices =
    let
      defaultMountOptions = [ "compress-force=zstd:1" ];
    in
    {
      disk = {
        main = {
          name = "main-a290ef43c9a346fda7bf7e45d3d1a965";
          device = "/dev/disk/by-id/ata-SK800-256GB_AA000000000000001697";
          type = "disk";
          content = {
            type = "gpt";
            partitions = {
              "boot" = {
                size = "1M";
                type = "EF02"; # for grub MBR
                priority = 1;
              };
              ESP = {
                type = "EF00";
                size = "500M";
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

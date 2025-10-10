_:
let
  defaultMountOptions = [ "compress-force=zstd" ];
in
{
  disko.devices = {
    disk = {
      vda = {
        type = "disk";
        device = "/dev/vda";
        content = {
          type = "gpt";
          partitions = {
            boot = {
              name = "boot";
              size = "1M";
              type = "EF02";
            };
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

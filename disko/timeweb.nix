{ ... }:
let defaultMountOptions = [ "compress-force=zstd:1" ];
in {
  disko.devices = {
    disk = {
      x = {
        type = "disk";
        device = "/dev/vda";
        content = {
          type = "gpt";
          partitions = {
            boot = {
              size = "1M";
              type = "EF02";
            };
            root = {
              size = "100%";
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
                  "/rootfs" = {
                    mountpoint = "/";
                    mountOptions = defaultMountOptions;
                  };
                  "/home" = {
                    mountpoint = "/home";
                    mountOptions = defaultMountOptions;
                  };
                  "/nix" = {
                    mountpoint = "/nix";
                    mountOptions = defaultMountOptions ++ [ "noatime" ];
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


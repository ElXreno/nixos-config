{ ... }: 
let 
  defaultMountOptions = [ "compress-force=zstd:1" ];
in
 {
  disko.devices = {
    disk = {
      x = {
        type = "disk";
        device = "/dev/sda";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              priority = 1;
              name = "ESP";
              start = "0";
              end = "512MiB";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
              };
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


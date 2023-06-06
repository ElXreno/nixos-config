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
          type = "table";
          format = "gpt";
          partitions = [
            {
              name = "boot";
              start = "0";
              end = "1M";
              flags = [ "bios_grub" ];
            }
            {
              name = "root";
              start = "1M";
              end = "100%";
              bootable = true;
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
                  "/rootfs" = {
                    mountpoint = "/";
                    mountOptions = defaultMountOptions;
                  };
                  "/home" = {
                    mountOptions = defaultMountOptions;
                  };
                  "/nix" = {
                    mountOptions = defaultMountOptions ++ [ "noatime" ];
                  };
                };
              };
            }
          ];
        };
      };
    };
  };
}


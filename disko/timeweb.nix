{ ... }: {
  disko.devices = {
    disk = {
      x = {
        type = "disk";
        device = "/dev/vda";
        content = {
          type = "gpt";
          partitions = {
            bios = {
              size = "1M";
              type = "EF02";
            };
            boot = {
              size = "512M";
              type = "EF00";
              content = {
                format = "vfat";
                mountOptions = [ "defaults" "umask=0077" ];
                mountpoint = "/boot";
                type = "filesystem";
              };
            };
            root = {
              size = "100%";
              content = {
                type = "filesystem";
                format = "bcachefs";
                extraArgs = [
                  "-f"
                  "--compression=lz4"
                  "--background_compression=zstd:12"
                ];
                mountpoint = "/";
              };
            };
          };
        };
      };
    };
  };
}


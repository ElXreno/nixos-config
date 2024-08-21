{ ... }: {
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
                type = "filesystem";
                format = "bcachefs";
                extraArgs = [
                  "-f"
                  "--compression=lz4"
                  "--background_compression=zstd:6"
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


_: {
  disko.devices = {
    disk = {
      nvme_trashbin = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-eui.0025384641baab15";
        content = {
          type = "gpt";
          partitions = {
            nvme-trashbin = {
              size = "100%";
              content = {
                type = "btrfs";
                extraArgs = [ "-f" ];
                subvolumes = {
                  "/trashbin" = {
                    mountpoint = "/mnt/trashbin";
                    mountOptions = [ "compress-force=zstd" ];
                  };
                  "/android" = {
                    mountpoint = "/mnt/android";
                    mountOptions = [ "compress-force=zstd" ];
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

_: {
  disko.devices = {
    disk = {
      nvme = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-eui.002538d722a0adfe";
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
              size = "100%";
              content = {
                type = "filesystem";
                format = "xfs";
                mountpoint = "/";
                mountOptions = [
                  "defaults"
                  "pquota"
                ];
              };
            };
          };
        };
      };
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

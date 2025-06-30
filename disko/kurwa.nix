_:
let
  defaultMountOptions = [ "compress-force=zstd" ];
in
{
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

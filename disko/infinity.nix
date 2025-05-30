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
              size = "200G";
              content = {
                type = "filesystem";
                format = "xfs";
                mountpoint = "/";
              };
            };
            luks = {
              size = "100%";
              content = {
                type = "luks";
                name = "crypted";
                extraOpenArgs = [ "--allow-discards" ];
                settings = {
                  allowDiscards = true;
                };
                content = {
                  type = "btrfs";
                  extraArgs = [ "-f" ];
                  subvolumes = {
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
  };
}

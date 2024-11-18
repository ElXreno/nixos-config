{ ... }:
let defaultMountOptions = [ "compress-force=zstd" ];
in {
  disko.devices = {
    disk = {
      nvme = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-eui.002538d722a0adfe";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "512MiB";
              type = "EF00";
              priority = 1;
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            luks = {
              size = "100%";
              content = {
                type = "luks";
                name = "crypted";
                extraOpenArgs = [ "--allow-discards" ];
                settings = { allowDiscards = true; };
                content = {
                  type = "btrfs";
                  extraArgs = [ "-f" ];
                  subvolumes = {
                    "/root" = {
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
                    "/var" = {
                      mountpoint = "/var";
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

  fileSystems = {
    "/var".neededForBoot =
      true; # Ensure that /var will be mounted with the sops key
  };
}


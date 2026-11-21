{
  disko.devices =
    let
      mountOptions = [
        "noatime"
        "compress=zstd:1"
      ];
    in
    {
      disk.userdata = {
        type = "disk";
        device = "/dev/disk/by-partlabel/userdata";
        content = {
          type = "btrfs";
          extraArgs = [ "-f" ];
          subvolumes = {
            "/root" = {
              mountpoint = "/";
              inherit mountOptions;
            };
            "/nix" = {
              mountpoint = "/nix";
              inherit mountOptions;
            };
            "/home" = {
              mountpoint = "/home";
              inherit mountOptions;
            };
          };
        };
      };
    };
}

{ namespace, ... }:

{
  imports = [ ./llm.nix ];

  ${namespace} = {
    nixpkgs.cudaSupport = true;

    system = {
      impermanence.enable = true;

      hardware = {
        gpu.nvidia = {
          overclock = {
            enable = true;
            freqOffset = 140;
            maxClock = 2625;
          };
          acpiNotifyRtd3Guard.enable = true;
        };
      };

      fs.zfs = {
        hostId = "790a20cd";
        autoSnapshot.filesystems = {
          "rpool/home-persist" = true;
          "rpool/root-persist<" = true;
        };
      };
    };

    services = {
      monitoring.prometheus.exporters.nvidia.enable = false;
    };

    home-manager.syncthing.randomPortIncrement = 42;
  };

  # I'll manage it manually
  # Nvidia kernel module too big for `/boot` partition
  hardware.facter.detected.graphics.enable = false;
}

{
  namespace,
  pkgs,
  config,
  ...
}:
{
  ${namespace} = {
    roles.server.enable = true;

    system = {
      boot.uefi.enable = true;

      hardware = {
        cpu.manufacturer = "intel";
      };

      networking.wireless.enable = true;
    };

    services = {
      ripe-atlas.enable = true;
      thermald.enable = true;
      thermald.configFile = ./thermal-conf.xml;
    };

    home-manager.syncthing.randomPortIncrement = 23;
  };

  boot.extraModulePackages = [
    (pkgs.callPackage ../../packages/mmio-fan {
      inherit (config.boot.kernelPackages) kernel;
    })
  ];

  boot.kernelModules = [ "mmio_fan" ];

  security.sudo.wheelNeedsPassword = false;
}

{
  namespace,
  pkgs,
  config,
  ...
}:
{
  # TODO: Move to the proper location
  hardware.graphics = {
    enable = true;
    extraPackages = with pkgs; [
      intel-media-driver
      intel-compute-runtime
    ];
  };

  environment.sessionVariables.LIBVA_DRIVER_NAME = "iHD";

  ${namespace} = {
    roles.server.enable = true;

    system = {
      boot.uefi.enable = true;

      networking.wireless.enable = true;
    };

    services = {
      nixflix.enable = true;
      ripe-atlas.enable = true;
      thermald.enable = true;
      thermald.configFile = ./thermal-conf.xml;

      home-assistant.enable = true;
    };

    home-manager.syncthing.randomPortIncrement = 23;
  };

  boot.extraModulePackages = [ config.boot.kernelPackages.mmio-fan ];

  boot.kernelModules = [ "mmio_fan" ];
}

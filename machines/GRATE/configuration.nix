{ namespace, ... }:
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
    };

    home-manager.syncthing.randomPortIncrement = 23;
  };

  security.sudo.wheelNeedsPassword = false;
}

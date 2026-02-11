{
  inputs,
  namespace,
  ...
}:

{
  imports = [
    ./disko-config.nix

    inputs.nixos-facter-modules.nixosModules.facter
    { config.facter.reportPath = ./facter.json; }
  ];

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

  system.stateVersion = "25.05";
}

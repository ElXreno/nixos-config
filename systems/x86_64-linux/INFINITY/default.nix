{
  namespace,
  ...
}:

{
  imports = [
    ./disko-config.nix
    ./hardware-configuration.nix
    ./wireguard.nix
  ];

  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    system = {
      autoupgrade.enable = true;
      boot.uefi.enable = true;
      hardware = {
        cpu.manufacturer = "amd";
        gpu.amd.enable = true;

        huawei-wmi.enable = true;
      };

      fs.zfs = {
        # enable = true;
        hostId = "20a7d5d8";
      };
    };

    user.alena.enable = true;

    desktop-environments.plasma.enable = true;

    home-manager.syncthing.randomPortIncrement = 66;
  };

  boot.extraModprobeConfig = ''
    options iwlwifi amsdu_size=3
    options iwlmvm power_scheme=1
  '';

  system.stateVersion = "25.05";
}

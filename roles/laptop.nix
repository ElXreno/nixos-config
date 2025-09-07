{ inputs, ... }:
{
  imports =
    with inputs.self.nixosModules;
    with inputs.self.nixosProfiles;
    [
      ./base.nix

      # Profiles
      hardware
      network

      home.programs.direnv
      home.programs.firefox
      home.programs.mangohud
      home.programs.mpv
      home.programs.vscodium
      home.services.syncthing

      programs.adb

      # services.ananicy
      services.printing
      services.restic
    ];

  deviceSpecific.isLaptop = true;
}

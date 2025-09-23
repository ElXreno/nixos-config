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
      home.programs.zed-editor
      home.services.syncthing

      programs.adb

      services.printing
      services.restic
    ];

  deviceSpecific.isLaptop = true;
}

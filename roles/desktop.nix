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

      home.programs.firefox
      home.programs.mpv
      home.services.syncthing

      programs.adb

      services.ananicy
      services.printing
    ];

  deviceSpecific.isDesktop = true;
}

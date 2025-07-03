{ inputs, ... }:
{
  imports =
    with inputs.self.nixosModules;
    with inputs.self.nixosProfiles;
    [
      ./base.nix

      # Profiles
      adb
      ananicy
      hardware
      firefox
      mpv
      network
      printing
      restic
      activitywatch
      syncthing
      vscodium
      direnv
      mangohud
    ];

  deviceSpecific.isLaptop = true;
}

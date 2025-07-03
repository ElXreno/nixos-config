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
      firefox
      hardware
      mpv
      network
      printing
      syncthing
    ];

  deviceSpecific.isDesktop = true;
}

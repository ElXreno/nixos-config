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
      flatpak
      firefox
      hardware
      minidlna
      mpv
      network
      printing
      syncthing
    ];

  device.isDesktop = true;
}

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
      hardware
      minidlna
      network
      printing
    ];

  device.isDesktop = true;
}

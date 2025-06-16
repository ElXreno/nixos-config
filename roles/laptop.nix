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
      # ipfs
      # minidlna
      network
      printing
      restic
    ];

  device.isLaptop = true;
}

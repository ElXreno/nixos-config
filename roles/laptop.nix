{ inputs, ... }: {
  imports = with inputs.self.nixosModules; with inputs.self.nixosProfiles; [
    ./base.nix

    # Profiles
    adb
    hardware
    firefox
    # ipfs
    # minidlna
    mpv
    network
    printing
    restic
    syncthing
    tlp
    vscodium
    direnv
  ];

  deviceSpecific.isLaptop = true;
}

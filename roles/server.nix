{ inputs, ... }: {
  imports = with inputs.self.nixosModules; with inputs.self.nixosProfiles; [
    ./base.nix

    # Profiles
    # fail2ban
    direnv
  ];

  deviceSpecific.isServer = true;
}

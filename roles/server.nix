{ inputs, ... }:
{
  imports =
    with inputs.self.nixosModules;
    with inputs.self.nixosProfiles;
    [
      ./base.nix

      # Profiles
      # fail2ban
      home.programs.direnv
    ];

  deviceSpecific.isServer = true;
}

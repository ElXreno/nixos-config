{ inputs, ... }:
{
  imports =
    with inputs.self.nixosModules;
    with inputs.self.nixosProfiles;
    [
      inputs.home-manager.nixosModules.home-manager

      # Modules
      devices

      # Profiles
      misc
      network
      nix
      overlay
      security
      sops

      home.programs.fish
      home.programs.git
      home.programs.gpg
      home.programs.htop
      home.programs.ssh
      home.programs.starship
      home.services.gpg-agent

      services.dnscrypt-proxy2
      services.openssh
    ];
}

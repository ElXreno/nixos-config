{ inputs, ... }: {
  imports = with inputs.self.nixosModules;
    with inputs.self.nixosProfiles; [
      inputs.home-manager.nixosModules.home-manager

      # Modules
      devices

      # Profiles
      dnscrypt-proxy2
      fish
      git
      gpg
      gpg-agent
      htop
      misc
      network
      nix
      openssh
      overlay
      security
      sops
      ssh
      starship
    ];
}

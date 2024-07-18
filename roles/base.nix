{ inputs, ... }: {
  imports = with inputs.self.nixosModules;
    with inputs.self.nixosProfiles; [
      inputs.home-manager.nixosModules.home-manager

      # Modules
      devices

      # Profiles
      boot
      common-packages
      dnscrypt-proxy2
      fish
      git
      gpg
      gpg-agent
      helix
      htop
      misc
      network
      nix
      nix-index
      openssh
      overlay
      security
      sops
      ssh
      starship
    ];
}

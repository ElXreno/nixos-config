{ inputs, ... }: {
  imports = with inputs.self.nixosModules; with inputs.self.nixosProfiles; [
    inputs.home-manager.nixosModules.home-manager

    # Modules
    devices

    # Profiles
    common-packages
    dnscrypt-proxy2
    fish
    git
    gpg
    gpg-agent
    htop
    misc
    network
    nix-index
    nix
    openssh
    overlay
    security
    sops
    ssh
    starship
  ];
}

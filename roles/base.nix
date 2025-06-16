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
      boot
      common-packages
      dnscrypt-proxy2
      command-not-found
      misc
      network
      nix
      openssh
      overlay
      security
      sops
    ];
}

{ inputs, ... }:

{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/scan/not-detected.nix"
    "${inputs.nixpkgs}/nixos/modules/profiles/qemu-guest.nix"
    inputs.disko.nixosModules.disko
    inputs.self.diskoConfigurations.hcloud
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.hydra
    inputs.self.nixosProfiles.attic
    inputs.self.nixosProfiles.attic-watch-store
    inputs.self.nixosProfiles.builder
  ];

  boot.binfmt.emulatedSystems = [ "x86_64-linux" ];

  security.sudo.wheelNeedsPassword = false;

  networking.useDHCP = false;
  networking.useNetworkd = true;
  systemd.network = {
    enable = true;
    networks = {
      "enp1s0" = {
        matchConfig.Name = "enp1s0";
        networkConfig.DHCP = "ipv4";
        address = [ "2a01:4f8:c2c:eaea::1/64" ];
        gateway = [ "fe80::1" ];
      };
    };
  };

  system.stateVersion = "24.11";
}

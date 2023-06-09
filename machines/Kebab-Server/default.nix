{ config, inputs, pkgs, lib, ... }:

{
  imports = [
    ./wireguard.nix
    "${inputs.nixpkgs}/nixos/modules/installer/scan/not-detected.nix"
    "${inputs.nixpkgs}/nixos/modules/profiles/qemu-guest.nix"
    inputs.disko.nixosModules.disko
    inputs.self.diskoConfigurations.hcloud
    inputs.self.nixosRoles.server
    (import inputs.self.nixosProfiles.k8s-master {
      inherit pkgs lib;
      kubeMasterHostname = config.device;
      kubeMasterIP = "100.103.121.36";
    })
  ];

  deviceSpecific.devInfo.legacy = true;

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  networking.firewall.allowedTCPPorts = [ 80 443 ];

  system.stateVersion = "23.05";
}

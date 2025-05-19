{ inputs, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix
    ./wireguard.nix
    inputs.disko.nixosModules.disko
    inputs.nixos-hardware.nixosModules.common-cpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.lanzaboote.nixosModules.lanzaboote
    inputs.self.diskoConfigurations.infinity
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.kde
    # inputs.self.nixosProfiles.hyprland
    # inputs.self.nixosProfiles.gamemode
    inputs.self.nixosProfiles.zfs
    # inputs.self.nixosProfiles.harmonia
    inputs.self.nixosProfiles.virtualisation
    # inputs.self.nixosProfiles.system76-scheduler
    # Lazy to configure everything from zero
    # inputs.self.nixosProfiles.sway
  ];

  # SecureBoot
  boot = {
    bootspec.enable = true;
    loader.systemd-boot.enable = lib.mkForce false;
    lanzaboote = {
      enable = true;
      pkiBundle = "/etc/secureboot";
    };
  };

  boot.extraModprobeConfig = ''
    # Disable hardware watchdog
    blacklist sp5100_tco

    options iwlwifi amsdu_size=3
    options iwlmvm power_scheme=1
  '';

  boot.binfmt.emulatedSystems = [ "aarch64-linux" ];

  hardware.amdgpu = {
    amdvlk.enable = true;
    initrd.enable = true;
    opencl.enable = true;
  };

  # P2P Networking
  networking.firewall.allowedTCPPorts = [ 36645 ];
  networking.firewall.allowedUDPPorts = [ 36645 ];

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  programs.noisetorch.enable = true;
  services.bpftune.enable = true;

  # Required sometimes
  # services.timesyncd.enable = false;

  system.stateVersion = "25.05";
}

{ inputs, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix
    ./wireguard.nix
    inputs.disko.nixosModules.disko
    inputs.nixos-hardware.nixosModules.common-cpu-amd
    inputs.nixos-hardware.nixosModules.common-cpu-amd-pstate
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.lanzaboote.nixosModules.lanzaboote
    inputs.self.diskoConfigurations.infinity
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.gamemode
    inputs.self.nixosProfiles.zfs
    inputs.self.nixosProfiles.harmonia
    # inputs.self.nixosProfiles.system76-scheduler
    # Lazy to configure everything from zero
    # inputs.self.nixosProfiles.sway
  ];

  swapDevices = [{
    device = "/swapfile";
  }];
  zramSwap.enable = false;
  boot.kernelParams = [
    "zswap.enabled=1"
    "zswap.compressor=zstd"
    "zswap.zpool=z3fold"
    "zswap.max_pool_percent=5"
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
  '';

  boot.binfmt.emulatedSystems = [ "aarch64-linux" ];

  # P2P Networking
  networking.firewall.allowedTCPPorts = [ 36645 ];
  networking.firewall.allowedUDPPorts = [ 36645 ];

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  programs.noisetorch.enable = true;
  services.tailscale.enable = true;
  services.bpftune.enable = true;

  services.postgresql = {
    enable = true;
    ensureDatabases = [ "l0_test" ];
    package = pkgs.postgresql_15;
  };

  system.stateVersion = "23.05";
}

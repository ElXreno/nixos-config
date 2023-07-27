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
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.gamemode
    inputs.self.nixosProfiles.virtualisation
    # inputs.self.nixosProfiles.zfs
    inputs.self.nixosProfiles.harmonia
    inputs.self.nixosProfiles.system76-scheduler
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

    # enable power savings mode of snd_hda_intel
    options snd-hda-intel power_save=1 power_save_controller=y

    options iwlwifi amsdu_size=3
  '';

  boot.kernelParams = [ "amd_pstate=active" ];

  # EPP cannot be set under performance policy
  # so use powersave by default
  powerManagement.cpuFreqGovernor = "powersave";

  # P2P Networking
  networking.firewall.allowedTCPPorts = [ 36645 ];
  networking.firewall.allowedUDPPorts = [ 36645 ];

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  programs.k3b.enable = true;
  programs.noisetorch.enable = true;
  services.tailscale.enable = true;
  services.bpftune.enable = true;

  services.beesd.filesystems = {
    root = {
      spec = "UUID=02f76e23-8371-4a97-a3c6-75fe096d931c";
      workDir = "beeshome";
    };
  };
  systemd.services."beesd@root".wantedBy = lib.mkForce [ ];

  services.postgresql = {
    enable = true;
    ensureDatabases = [ "l0_test" ];
    package = pkgs.postgresql_15;
  };

  system.stateVersion = "23.05";
}

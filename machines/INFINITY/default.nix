{ inputs, lib, ... }:

{
  imports = [
    ./graduate-project-dev.nix
    ./hardware-configuration.nix
    ./wireguard.nix
    inputs.nixos-hardware.nixosModules.common-cpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.lanzaboote.nixosModules.lanzaboote
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
    # enable power savings mode of snd_hda_intel
    options snd-hda-intel power_save=1 power_save_controller=y

    options iwlwifi amsdu_size=3
  '';

  boot.kernelParams = [ "amd_pstate=active" ];

  # EPP cannot be set under performance policy
  # so use powersave by default
  powerManagement.cpuFreqGovernor = "powersave";

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  services.tailscale.enable = true;

  system.stateVersion = "22.05";
}

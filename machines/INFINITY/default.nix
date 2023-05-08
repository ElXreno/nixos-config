{ config, inputs, lib, ... }:

let
  cifsOptions = [
    "actimeo=300"
    "cache=loose"
    "noauto"
    "x-systemd.automount"
    "x-systemd.device-timeout=5s"
    "x-systemd.idle-timeout=60"
    "x-systemd.mount-timeout=5s"
  ];
  mkMountPath = prefix: path: "/mnt/${prefix}/${path}";
  mkDevicePath = addr: path: "//${addr}/${path}";
  mkCifsFilesystem = path: prefix: addr: credentials: {
    name = "${mkMountPath prefix path}";
    value = {
      device = mkDevicePath addr path;
      fsType = "cifs";
      options = cifsOptions ++ lib.optional (credentials != null) "credentials=${credentials}";
    };
  };
in
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
    inputs.self.nixosProfiles.nix-serve
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

  sops.secrets."smb/college" = { };
  fileSystems =
    let collegeCifs = map
      (path: mkCifsFilesystem path "college" "10.1.37.4" config.sops.secrets."smb/college".path)
      [ "Study" "Install" "Data" ];
    in builtins.listToAttrs collegeCifs;

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  services.tailscale.enable = true;

  system.stateVersion = "22.05";
}

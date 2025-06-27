{
  config,
  inputs,
  lib,
  pkgs,
  ...
}:

{
  imports = [
    ./hardware-configuration.nix
    ./wireguard.nix
    inputs.disko.nixosModules.disko
    inputs.nixos-hardware.nixosModules.common-cpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-nvidia-ada-lovelace
    inputs.nixos-hardware.nixosModules.common-gpu-nvidia-prime
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.lanzaboote.nixosModules.lanzaboote
    inputs.self.diskoConfigurations.kurwa
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.zfs
    # inputs.self.nixosProfiles.harmonia
    inputs.self.nixosProfiles.virtualisation
  ];

  boot.extraModprobeConfig = ''
    options rtw89_core disable_ps_mode=Y
  '';

  hardware.amdgpu = {
    amdvlk.enable = true;
    initrd.enable = true;
    opencl.enable = true;
  };

  services = {
    asusd = {
      enable = lib.mkDefault true;
      enableUserService = lib.mkDefault true;
    };
  };

  services.xserver.videoDrivers = [
    "amdgpu"
    "nvidia"
  ];

  boot.kernelParams = [
    "nvidia.NVreg_EnableS0ixPowerManagement=1"
    "nvidia.NVreg_DynamicPowerManagement=0x01"
  ];
  hardware.nvidia = {
    package = config.boot.kernelPackages.nvidiaPackages.production;
    dynamicBoost.enable = lib.mkDefault true;
    prime = {
      amdgpuBusId = "PCI:66:0:0";
      nvidiaBusId = "PCI:1:0:0";
    };
    powerManagement.enable = true;
    primeBatterySaverSpecialisation = true;
  };

  specialisation.battery-saver.configuration = {
    hardware.nvidia.dynamicBoost.enable = false;
  };

  programs.nix-ld.enable = true;
  programs.steam = {
    enable = true;
    remotePlay.openFirewall = true;
    dedicatedServer.openFirewall = true;
    localNetworkGameTransfers.openFirewall = true;
  };
  programs.noisetorch.enable = true;
  services.bpftune.enable = true;

  # Required sometimes
  # services.timesyncd.enable = false;

  nix.settings.system-features = [ "gccarch-znver4" ];

  system.stateVersion = "25.05";
}

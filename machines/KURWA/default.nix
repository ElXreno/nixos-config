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
    inputs.disko.nixosModules.disko
    inputs.nixos-hardware.nixosModules.common-cpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-nvidia
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
  ];
  hardware.nvidia = {
    package = config.boot.kernelPackages.nvidiaPackages.latest;
    open = true;
    dynamicBoost.enable = true;
    prime = {
      amdgpuBusId = "PCI:66:0:0";
      nvidiaBusId = "PCI:1:0:0";
    };
    powerManagement.enable = true;
    powerManagement.finegrained = true;
    primeBatterySaverSpecialisation = true;
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

  system.stateVersion = "25.05";
}

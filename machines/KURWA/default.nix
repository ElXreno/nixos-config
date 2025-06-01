{
  inputs,
  lib,
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
    inputs.self.diskoConfigurations.infinity
    inputs.self.nixosRoles.laptop
    # inputs.self.nixosProfiles.tailscale
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
  hardware.nvidia.open = true;

  hardware.nvidia.prime = {
    amdgpuBusId = "PCI:65:0:0";
    nvidiaBusId = "PCI:1:0:0";
  };

  hardware.nvidia.primeBatterySaverSpecialisation = true;

  # Crashes GPU sometimes, will investigate later
  # hardware.nvidia.powerManagement.enable = true;
  hardware.nvidia.powerManagement.finegrained = true;

  hardware.nvidia.dynamicBoost.enable = true;

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  programs.noisetorch.enable = true;
  services.bpftune.enable = true;

  # Required sometimes
  # services.timesyncd.enable = false;

  system.stateVersion = "25.05";
}

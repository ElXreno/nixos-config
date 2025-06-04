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
    "nvidia.NVreg_RegistryDwords=PowerMizerEnable=0x1;PerfLevelSrc=0x2222;PowerMizerLevel=0x3;PowerMizerDefault=0x3;PowerMizerDefaultAC=0x3"
  ];
  boot.kernelPatches =
    let
      asus-armoury = pkgs.fetchurl {
        url = "https://github.com/CachyOS/kernel-patches/raw/20175136fee6e725efc5940b141d45b4f8cd19d2/6.14/0003-asus.patch";
        hash = "sha256-pc/DCcC5TxZsy5jluK0PYpWmdNtVJ7jadhwhMybdqiI=";
      };
    in
    [
      {
        name = "asus-armoury";
        patch = asus-armoury;
        extraStructuredConfig.ASUS_ARMOURY = lib.kernel.module;
      }
    ];
  hardware.nvidia = {
    package = config.boot.kernelPackages.nvidiaPackages.latest;
    open = true;
    prime = {
      amdgpuBusId = "PCI:66:0:0";
      nvidiaBusId = "PCI:1:0:0";
    };
    powerManagement.enable = true;
    powerManagement.finegrained = true;
    primeBatterySaverSpecialisation = true;
  };

  programs.nix-ld.enable = true;
  programs.steam.enable = true;
  programs.noisetorch.enable = true;
  services.bpftune.enable = true;

  # Required sometimes
  # services.timesyncd.enable = false;

  system.stateVersion = "25.05";
}

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
    inputs.nixos-hardware.nixosModules.asus-battery
    inputs.lanzaboote.nixosModules.lanzaboote
    inputs.self.diskoConfigurations.kurwa
    inputs.self.diskoConfigurations.kurwa-trashbin
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.zfs
    # inputs.self.nixosProfiles.harmonia
    inputs.self.nixosProfiles.virtualisation
  ];

  boot.extraModprobeConfig = ''
    options rtw89_core disable_ps_mode=Y
    options nvidia NVreg_EnableS0ixPowerManagement=1 NVreg_DynamicPowerManagement=0x01
  '';

  hardware.amdgpu = {
    opencl.enable = true;
  };

  services = {
    asusd = {
      enable = lib.mkDefault true;
      enableUserService = lib.mkDefault true;
    };
  };

  services.xserver.videoDrivers = [ "nvidia" ];

  hardware = {
    asus.battery.chargeUpto = lib.mkDefault 70;

    nvidia = {
      package = config.boot.kernelPackages.nvidiaPackages.latest;
      dynamicBoost.enable = lib.mkDefault true;
      prime = {
        amdgpuBusId = "PCI:66:0:0";
        nvidiaBusId = "PCI:1:0:0";
      };
      powerManagement.enable = true;
      nvidiaPersistenced = true;
      primeBatterySaverSpecialisation = true;
    };
  };

  systemd.services = {
    nvidia_oc = lib.mkIf (config.specialisation != { }) {
      description = "NVIDIA Overclocking Service";
      after = [ "graphical.target" ];
      wantedBy = [ "graphical.target" ];

      serviceConfig = {
        ExecStart = "${pkgs.nvidia_oc}/bin/nvidia_oc set --index 0 --freq-offset 120 --min-clock 210 --max-clock 2655 --mem-offset 1150";
        User = "root";
        Restart = "on-failure";
      };
    };

    nvidia-powerd.serviceConfig = {
      Restart = "always";
    };
  };

  specialisation.battery-saver.configuration = {
    hardware = {
      asus.battery.chargeUpto = 95;
      nvidia.dynamicBoost.enable = false;
    };
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

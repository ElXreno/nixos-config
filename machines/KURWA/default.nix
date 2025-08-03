{
  config,
  inputs,
  lib,
  pkgs,
  ...
}:

{
  imports = [
    ./wireguard.nix

    inputs.nixos-facter-modules.nixosModules.facter
    { config.facter.reportPath = ./facter.json; }

    inputs.disko.nixosModules.disko
    inputs.nixos-hardware.nixosModules.common-cpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-nvidia
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.lanzaboote.nixosModules.lanzaboote
    inputs.self.diskoConfigurations.kurwa
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.services.asusd
    inputs.self.nixosProfiles.services.supergfxd
    inputs.self.nixosProfiles.services.tailscale
    inputs.self.nixosProfiles.services.monitoring
    inputs.self.nixosProfiles.services.sing-box-client
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.zfs
    inputs.self.nixosProfiles.virtualisation
  ];

  # I'll manage it manually
  # Nvidia kernel module too big for `/boot` partition
  facter.detected.graphics.enable = false;

  # Workaround for https://www.reddit.com/r/Fedora/comments/1gystaj/amdgpu_dmcub_error_collecting_diagnostic_data/
  boot.kernelParams = [
    "amdgpu.dcdebugmask=0x10"
  ];

  boot.extraModprobeConfig = ''
    options rtw89_core disable_ps_mode=Y
    options nvidia NVreg_EnableS0ixPowerManagement=1 NVreg_DynamicPowerManagement=0x01
  '';

  hardware.amdgpu = {
    opencl.enable = true;
  };

  services = {
    xserver.videoDrivers = [
      "amdgpu"
      "nvidia"
    ];

    beesd.filesystems = lib.mkIf (config.specialisation != { }) {
      "root" = {
        spec = "PARTLABEL=disk-nvme-root";
        hashTableSizeMB = 4096;
      };
    };

    ollama = lib.mkIf (config.specialisation != { }) {
      enable = true;
      acceleration = "cuda";
    };
  };

  hardware = {
    nvidia = {
      open = true;
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
  hardware.xpadneo.enable = true; # Xbox controller

  # Required sometimes
  # services.timesyncd.enable = false;

  nix.settings.system-features = [ "gccarch-znver4" ];

  system.stateVersion = "25.05";
}

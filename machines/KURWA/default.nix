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
    inputs.nixos-hardware.nixosModules.common-cpu-amd-zenpower
    inputs.nixos-hardware.nixosModules.common-gpu-amd
    inputs.nixos-hardware.nixosModules.common-gpu-nvidia
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.self.diskoConfigurations.kurwa
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.services.asusd
    inputs.self.nixosProfiles.services.supergfxd
    inputs.self.nixosProfiles.services.tailscale
    inputs.self.nixosProfiles.services.sing-box-client
    # inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.hyprland
    # inputs.self.nixosProfiles.zfs
    inputs.self.nixosProfiles.virtualisation
    # inputs.self.nixosProfiles.services.monitoring
    inputs.self.nixosProfiles.services.postgresql
  ];

  # I'll manage it manually
  # Nvidia kernel module too big for `/boot` partition
  facter.detected.graphics.enable = false;
  # And dhcp too
  facter.detected.dhcp.enable = false;

  # Workaround for https://www.reddit.com/r/Fedora/comments/1gystaj/amdgpu_dmcub_error_collecting_diagnostic_data/
  boot.kernelParams = [ "amdgpu.dcdebugmask=0x10" ];

  boot.extraModprobeConfig = ''
    options nvidia NVreg_EnableS0ixPowerManagement=1 NVreg_DynamicPowerManagement=0x01
  '';

  hardware.amdgpu.opencl.enable = true;
  services.scx = {
    enable = true;
    scheduler = "scx_lavd";
    extraArgs = [ "--autopower" ];
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
  };

  networking.firewall = {
    allowedTCPPorts = [
      25565 # Minecraft
      57621 # Spotify
      8100
    ];
    allowedUDPPorts = [
      25565 # Minecraft
    ];
  };

  systemd.services."beesd@root" = {
    wantedBy = lib.mkForce [ ];
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
      # nvidiaPersistenced = true;
    };
  };

  systemd.services = {
    nvidia_oc = {
      description = "NVIDIA Overclocking Service";
      after = [ "graphical.target" ];
      wantedBy = [ "graphical.target" ];

      serviceConfig = {
        ExecStart = "${pkgs.nvidia_oc}/bin/nvidia_oc set --index 0 --freq-offset 120 --min-clock 210 --max-clock 2655 --mem-offset 1150";
        User = "root";
        Restart = "on-failure";
      };
    };

    nvidia-powerd.serviceConfig.Restart = "always";
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

  services.bitmagnet = {
    enable = true;
    openFirewall = true;
    settings = {
      tmdb.enabled = false;
      processor.concurrency = 16;
      dht_crawler.scaling_factor = 50;
      dht_crawler.save_files_threshold = 1000;
    };
  };
  systemd.services.bitmagnet = {
    wantedBy = lib.mkForce [ ];
  };

  # Required sometimes
  # services.timesyncd.enable = false;

  system.stateVersion = "25.05";
}

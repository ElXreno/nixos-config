{
  inputs,
  namespace,
  virtual,
  ...
}:

{
  imports = [
    ./disko-config.nix
    ./wireguard.nix

    inputs.nixos-facter-modules.nixosModules.facter
    { config.facter.reportPath = ./facter.json; }
  ];

  ${namespace} = {
    roles.laptop.enable = true;

    system = {
      boot = {
        uefi.enable = true;
        kernel.optimizations = {
          enable = true;
          isa = 3;
        };
      };

      hardware = {
        asus.fa507uv.enable = true;
        cpu.manufacturer = "amd";
        gpu.amd.enable = true;
        gpu.nvidia = {
          enable = true;
          dynamicBoost.enable = true;
          overclock.enable = true;

          prime = {
            enable = true;
            amdgpuBusId = "PCI:66:0:0";
            nvidiaBusId = "PCI:1:0:0";
          };
        };

        lamzu.enable = true;
      };

      fs.zfs = {
        # enable = true;
        hostId = "e7e00058";
      };

      virtualisation = {
        libvirtd.enable = true;
        podman.enable = true;
      };
    };

    services = {
      asusd.enable = true;
      supergfxd.enable = true;

      sing-box.client.enable = true;
      # Slow downs network I/O when CPU is busy
      # scx.enable = true;
      postgresql.enable = true;
      bitmagnet.enable = true;
      restic.enable = !virtual;
      # monitoring.enable = true;

      pipewire.enableRNNoise = true;

      ollama = {
        # enable = true;
        acceleration = "rocm"; # Just benchmarked iGPU
      };
    };

    programs = {
      wireshark.enable = true;
      steam = {
        enable = true;
        xboxSupport = true;
      };
      noisetorch.enable = true;
    };

    desktop-environments.hyprland.enable = true;
  };

  sops.secrets."yandex-license" = {
    group = "users";
    mode = "0440";
  };

  # I'll manage it manually
  # Nvidia kernel module too big for `/boot` partition
  facter.detected.graphics.enable = false;
  # And dhcp too
  facter.detected.dhcp.enable = false;

  # Workaround for https://www.reddit.com/r/Fedora/comments/1gystaj/amdgpu_dmcub_error_collecting_diagnostic_data/
  boot.kernelParams = [ "amdgpu.dcdebugmask=0x10" ];

  boot.extraModprobeConfig = ''
    # NVIDIA dGPU
    options nvidia NVreg_EnableS0ixPowerManagement=1 NVreg_DynamicPowerManagement=0x01 NVreg_UsePageAttributeTable=1 NVreg_RegistryDwords="PowerMizerEnable=0x1; PowerMizerDefaultAC=0x3; PerfLevelSrc=0x3322;"
    options nvidia NVreg_EnableStreamMemOPs=1 NVreg_EnableResizableBar=1 NVreg_EnablePCIERelaxedOrderingMode=1

    # Realtek RTL8852BE
    options rtw89_core disable_ps_mode=Y
    options rtw89_pci disable_aspm_l1=y
  '';

  networking.firewall = {
    allowedTCPPorts = [
      25565 # Minecraft
      57621 # Spotify
      8100 # Minecraft web map
    ];
    allowedUDPPorts = [
      25565 # Minecraft
    ];
  };

  # Required sometimes
  # services.timesyncd.enable = false;

  system.stateVersion = "25.05";
}

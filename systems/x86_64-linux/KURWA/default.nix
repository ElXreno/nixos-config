{
  inputs,
  namespace,
  virtual,
  pkgs,
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
        uefi = {
          enable = true;
        };
        kernel.optimizations = {
          enable = true;
          isa = 3;
        };
      };

      impermanence.enable = true;

      hardware = {
        asus.fa507uv.enable = true;
        cpu.manufacturer = "amd";
        gpu.amd.enable = true;
        gpu.nvidia = {
          enable = true;
          dynamicBoost.enable = true;
          overclock.enable = true;
          powerManagement.finegrained = true;

          prime = {
            enable = true;
            amdgpuBusId = "PCI:66:0:0";
            nvidiaBusId = "PCI:1:0:0";
          };
        };

        lamzu.enable = true;
        fifine-am8 = {
          enable = true;
          pipewire.noise-cancelling.target = "alsa_input.usb-MV-SILICON_fifine_Microphone_20190808-00.analog-stereo";
        };
        silakka54.enable = true;
      };

      fs.zfs = {
        enable = true;
        hostId = "e7e00058";
      };

      virtualisation = {
        libvirtd.enable = true;
        podman.enable = true;
        docker.enable = true;
      };
    };

    services = {
      asusd.enable = true;
      supergfxd.enable = true;

      sing-box.client.enable = true;
      scx.enable = true;
      postgresql.enable = true;
      bitmagnet.enable = true;
      restic.enable = !virtual;
      # monitoring.enable = true;

      pipewire.rnnoise = {
        enable = true;
        target = "alsa_input.pci-0000_66_00.6.analog-stereo";
      };

      flatpak.enable = true;
      modprobed-db.enable = true;
    };

    programs = {
      wireshark.enable = true;
      obs-studio.enable = true;
      steam = {
        enable = true;
        xboxSupport = true;
      };
      noisetorch.enable = true;
    };

    desktop-environments.niri.enable = true;

    home-manager.syncthing.randomPortIncrement = 42;
  };

  # sops.secrets."yandex-license" = {
  #   group = "users";
  #   mode = "0440";
  # };

  # I'll manage it manually
  # Nvidia kernel module too big for `/boot` partition
  facter.detected.graphics.enable = false;
  # And dhcp too
  facter.detected.dhcp.enable = false;

  boot.kernelParams = [
    "acpi_osi=!"
    "acpi_osi=\"Windows 2022\""
    "tsc=nowatchdog"

    "pcie_aspm.policy=powersupersave"
  ];

  boot.extraModprobeConfig = ''
    # NVIDIA dGPU
    options nvidia NVreg_EnableS0ixPowerManagement=1 NVreg_UsePageAttributeTable=1
    options nvidia NVreg_EnableStreamMemOPs=1 NVreg_EnableResizableBar=1 NVreg_EnablePCIERelaxedOrderingMode=1

    # MT7925
    options mt7925e disable_aspm=y
  '';

  networking.firewall = {
    allowedTCPPorts = [
      25565 # Minecraft
      57621 # Spotify
      8100 # Minecraft web map
      56660 # Torrents
    ];
    allowedUDPPorts = [
      25565 # Minecraft
      56660 # Torrents
    ];
  };

  # Required sometimes
  # services.timesyncd.enable = false;

  system.stateVersion = "25.05";
}

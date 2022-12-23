{ config, inputs, pkgs, lib, ... }:

{
  imports = with inputs.self.nixosProfiles;
    [
      ./hardware-configuration.nix
      ./wireguard.nix
      inputs.nixos-hardware.nixosModules.common-cpu-amd
      inputs.nixos-hardware.nixosModules.common-cpu-amd-pstate
      inputs.nixos-hardware.nixosModules.common-gpu-amd
      inputs.nixos-hardware.nixosModules.common-pc-laptop
      inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
      inputs.self.nixosRoles.laptop
      inputs.self.nixosProfiles.kde
      # Lazy to configure everything from zero
      # inputs.self.nixosProfiles.sway
    ];

  networking.hostId = "20a7d5d8";
  boot.supportedFilesystems = [ "zfs" ];

  boot.extraModprobeConfig = ''
    # enable power savings mode of snd_hda_intel
    options snd-hda-intel power_save=1 power_save_controller=y
  '';

  services.tailscale.enable = true;

  powerManagement.cpuFreqGovernor = "performance";

  sops.secrets."smb/college" = { };
  fileSystems."/mnt/college/Study" = {
    device = "//10.1.37.4/Study";
    fsType = "cifs";
    options = [
      "actimeo=300"
      "cache=loose"
      "credentials=${config.sops.secrets."smb/college".path}"
      "noauto"
      "x-systemd.automount"
      "x-systemd.device-timeout=5s"
      "x-systemd.idle-timeout=60"
      "x-systemd.mount-timeout=5s"
    ];
  };

  fileSystems."/mnt/college/Install" = {
    device = "//10.1.37.4/Install";
    fsType = "cifs";
    options = [
      "actimeo=300"
      "cache=loose"
      "credentials=${config.sops.secrets."smb/college".path}"
      "noauto"
      "x-systemd.automount"
      "x-systemd.device-timeout=5s"
      "x-systemd.idle-timeout=60"
      "x-systemd.mount-timeout=5s"
    ];
  };

  fileSystems."/mnt/college/Data" = {
    device = "//10.1.37.4/Data";
    fsType = "cifs";
    options = [
      "actimeo=300"
      "cache=loose"
      "credentials=${config.sops.secrets."smb/college".path}"
      "noauto"
      "x-systemd.automount"
      "x-systemd.device-timeout=5s"
      "x-systemd.idle-timeout=60"
      "x-systemd.mount-timeout=5s"
    ];
  };

  fileSystems."/mnt/college/Control-works" = {
    device = "//10.1.37.4/Контрольные работы";
    fsType = "cifs";
    options = [
      "actimeo=300"
      "cache=loose"
      "credentials=${config.sops.secrets."smb/college".path}"
      "noauto"
      "x-systemd.automount"
      "x-systemd.device-timeout=5s"
      "x-systemd.idle-timeout=60"
      "x-systemd.mount-timeout=5s"
    ];
  };

  # For shindows
  time.hardwareClockInLocalTime = true;

  boot.loader.efi.canTouchEfiVariables = true;

  boot.loader.grub = {
    device = lib.mkForce "nodev";
    efiSupport = true;
    enableCryptodisk = true;
  };
  boot.loader.efi.efiSysMountPoint = "/boot/efi";

  boot.binfmt.emulatedSystems = [ "aarch64-linux" ];
  virtualisation = {
    spiceUSBRedirection.enable = true;
    libvirtd = {
      enable = true;
      qemu.package = pkgs.qemu_kvm;
    };
  };

  services.yggdrasil = {
    # enable = true;
    persistentKeys = true;
    openMulticastPort = true;
    settings = {
      Peers = [
        "tcp://51.15.118.10:62486"
        "tls://45.147.198.155:6010"
        "tls://77.95.229.240:62486"
        "tls://94.103.82.150:8080"
        "tls://ygg-nl.incognet.io:8884"
      ];
    };
  };

  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="sd[a-z]*[0-9]*|mmcblk[0-9]*p[0-9]*|nvme[0-9]*n[0-9]*p[0-9]*", ENV{ID_FS_TYPE}=="zfs_member", ATTR{../queue/scheduler}="none"
  '';

  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_15;
    authentication = pkgs.lib.mkOverride 10 ''
      local all all trust
      host all all 127.0.0.1/32 trust
      host all all ::1/128 trust
    '';
  };

  programs.gamemode = {
    enable = true;
    settings = {
      general = {
        renice = 10;
      };

      gpu = {
        apply_gpu_optimisations = "accept-responsibility";
        gpu_device = 0;
        amd_performance_level = "high";
      };

      custom = {
        start = "${pkgs.libnotify}/bin/notify-send 'GameMode started'";
        end = "${pkgs.libnotify}/bin/notify-send 'GameMode ended'";
      };
    };
  };

  programs.nix-ld.enable = true;

  system.stateVersion = "22.05";
}

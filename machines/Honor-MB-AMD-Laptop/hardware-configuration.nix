{ config, lib, pkgs, modulesPath, ... }:

{
  imports =
    [
      (modulesPath + "/installer/scan/not-detected.nix")
    ];

  boot.initrd.availableKernelModules = [ "nvme" "xhci_pci" "uas" "sd_mod" "tpm" ];
  boot.initrd.kernelModules = [ ];
  boot.kernelModules = [ "kvm-amd" ];
  boot.extraModulePackages = [ ];

  fileSystems."/" =
    {
      device = "/dev/disk/by-uuid/745f1c6e-8368-4f92-820b-f4a18086b808";
      fsType = "btrfs";
      options = [ "subvol=root" "compress-force=zstd" "discard=async" ];
    };

  boot.initrd.luks.devices."cryptroot" = {
    device = "/dev/disk/by-uuid/656eb0a6-4ec2-4dc9-a6f7-5fa531c75d8c";
    allowDiscards = true;
  };

  fileSystems."/home" =
    {
      device = "/dev/disk/by-uuid/745f1c6e-8368-4f92-820b-f4a18086b808";
      fsType = "btrfs";
      options = [ "subvol=home" "compress-force=zstd" "discard=async" ];
    };

  fileSystems."/nix" =
    {
      device = "/dev/disk/by-uuid/745f1c6e-8368-4f92-820b-f4a18086b808";
      fsType = "btrfs";
      options = [ "subvol=nix" "compress-force=zstd" "discard=async" ];
    };

  fileSystems."/boot" =
    {
      device = "/dev/disk/by-uuid/c2c4b6b3-b0b3-45a8-a508-256843c3fb52";
      fsType = "ext4";
    };

  fileSystems."/boot/efi" =
    {
      device = "/dev/disk/by-uuid/5BD1-0742";
      fsType = "vfat";
    };

  swapDevices = [ ];

  hardware.cpu.amd.updateMicrocode = lib.mkDefault config.hardware.enableRedistributableFirmware;
}

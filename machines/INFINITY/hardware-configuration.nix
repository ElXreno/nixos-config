# Do not modify this file!  It was generated by ‘nixos-generate-config’
# and may be overwritten by future invocations.  Please make changes
# to /etc/nixos/configuration.nix instead.
{ config, lib, pkgs, modulesPath, ... }:

{
  imports =
    [ (modulesPath + "/installer/scan/not-detected.nix")
    ];

  boot.initrd.availableKernelModules = [ "nvme" "xhci_pci" "usb_storage" "sd_mod" ];
  boot.initrd.kernelModules = [ ];
  boot.kernelModules = [ "kvm-amd" ];
  boot.extraModulePackages = [ ];

  fileSystems."/" =
    { device = "/dev/disk/by-uuid/0de70103-ee93-4976-9dc1-a16237005c4e";
      fsType = "btrfs";
      options = [ "subvol=nixos-root" "compress-force=zstd" "discard=async" ];
    };

  boot.initrd.luks.devices."nvme-luks" = {
    device = "/dev/disk/by-uuid/4191c902-7b14-49fc-9550-7d75c1887e86";
    allowDiscards = true;
  };

  fileSystems."/home" =
    { device = "/dev/disk/by-uuid/0de70103-ee93-4976-9dc1-a16237005c4e";
      fsType = "btrfs";
      options = [ "subvol=nixos-home" "compress-force=zstd" "discard=async" ];
    };

  fileSystems."/nix" =
    { device = "/dev/disk/by-uuid/0de70103-ee93-4976-9dc1-a16237005c4e";
      fsType = "btrfs";
      options = [ "subvol=nix" "compress-force=zstd" "discard=async" ];
    };


  fileSystems."/boot" =
    { device = "/dev/disk/by-uuid/4e43791f-f4be-4272-a87d-d6f146defaca";
      fsType = "ext4";
    };

  fileSystems."/boot/efi" =
    { device = "/dev/disk/by-uuid/FE41-CBB2";
      fsType = "vfat";
    };

  swapDevices = [ ];

  # Enables DHCP on each ethernet and wireless interface. In case of scripted networking
  # (the default) this is the recommended approach. When using systemd-networkd it's
  # still possible to use this option, but it's recommended to use it in conjunction
  # with explicit per-interface declarations with `networking.interfaces.<interface>.useDHCP`.
  networking.useDHCP = lib.mkDefault true;
  # networking.interfaces.wlp1s0.useDHCP = lib.mkDefault true;

  nixpkgs.hostPlatform = lib.mkDefault "x86_64-linux";
  hardware.cpu.amd.updateMicrocode = lib.mkDefault config.hardware.enableRedistributableFirmware;
}
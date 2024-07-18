{ inputs, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix
    inputs.nixos-hardware.nixosModules.common-cpu-intel-sandy-bridge
    inputs.nixos-hardware.nixosModules.common-gpu-nvidia-disable
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-acpi_call
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.kde
    inputs.impermanence.nixosModules.impermanence
  ];

  deviceSpecific.devInfo.legacy = true;

  boot.kernelPackages = lib.mkForce pkgs.linuxPackages_xanmod;

  # services.xserver.videoDrivers = [ "nvidia" ];
  # hardware.nvidia = {
  #   package = config.boot.kernelPackages.nvidiaPackages.legacy_390;
  #   prime = {
  #     sync.enable = true;
  #     nvidiaBusId = "PCI:1:0:0";
  #     intelBusId = "PCI:0:2:0";
  #   };
  # };
  # services.xserver.extraConfig = ''
  #   Section "Monitor"
  #     Identifier "VGA-0"
  #     Option "Ignore" "true"
  #     Option "Enable" "false"
  #   EndSection
  # '';

  # Fix i915 crash after suspend
  systemd.services = {
    "rmmod-iwlwifi-before-suspend" = {
      description = "Unload iwlwifi kernel module before suspend";
      wantedBy = [ "suspend.target" ];
      before = [ "systemd-suspend.service" ];
      script = ''
        ${pkgs.systemd}/bin/systemctl stop iwd NetworkManager
        ${pkgs.kmod}/bin/modprobe -r iwldvm iwlwifi

        # Test the second work-around for less frequent iwlwifi crash
        ${pkgs.coreutils}/bin/sleep 3
      '';
      serviceConfig.Type = "simple";
    };
    "insmod-iwlwifi-after-suspend" = {
      description = "Load iwlwifi kernel module after suspend";
      wantedBy = [ "suspend.target" ];
      after = [ "systemd-suspend.service" ];
      script = ''
        # Test the second work-around for less frequent iwlwifi crash
        ${pkgs.coreutils}/bin/sleep 3

        ${pkgs.kmod}/bin/modprobe iwlwifi
        ${pkgs.systemd}/bin/systemctl start iwd NetworkManager
      '';
      serviceConfig.Type = "simple";
    };
    # "bbswitch".serviceConfig = builtins.removeAttrs (config.systemd.services."bbswitch".serviceConfig) [ "ExecStart" ];
  };

  # ddnet server
  networking.firewall.allowedTCPPorts = [ 8303 ];
  networking.firewall.allowedUDPPorts = [ 8303 ];

  virtualisation = {
    spiceUSBRedirection.enable = true;
    libvirtd = {
      enable = true;
      qemu.package = pkgs.qemu_kvm;
    };
  };

  environment.persistence."/persist/root" = {
    directories = [
      "/etc/NetworkManager/system-connections"
      "/etc/nixos"
      "/var/cache"
      "/var/lib/bluetooth"
      "/var/lib/iwd"
      "/var/lib/postgresql"
      "/var/lib/systemd"
      "/var/lib/libvirt"
      "/var/lib/tailscale"
      "/var/log"
    ];
    files = [
      "/etc/machine-id"
      "/etc/ssh/ssh_host_ed25519_key"
      "/etc/ssh/ssh_host_ed25519_key.pub"
      "/etc/ssh/ssh_host_rsa_key"
      "/etc/ssh/ssh_host_rsa_key.pub"
    ];
  };

  # For Shindows
  # time.hardwareClockInLocalTime = true;

  services.beesd.filesystems = {
    root = {
      spec = "UUID=39d0c00d-05e6-4e0b-9bd9-9ee9f45afd90";
      # verbosity = "info";
      workDir = "beeshome";
      # extraOptions = [ "--thread-count" "2" ];
    };
  };
  systemd.services."beesd@root" = {
    wantedBy = lib.mkForce [ ];
    serviceConfig = {
      CPUSchedulingPolicy = lib.mkForce "idle";
      IOSchedulingClass = "idle";
    };
  };

  services.udev.extraRules = ''
    ACTION=="change", SUBSYSTEM=="power_supply", ATTR{online}=="0", RUN+="${pkgs.systemd}/bin/systemctl stop beesd@root"
    ACTION=="change", SUBSYSTEM=="power_supply", ATTR{online}=="1", RUN+="${pkgs.systemd}/bin/systemctl start beesd@root"
  '';

  # services.tailscale.enable = true;

  programs.steam.enable = true;

  system.stateVersion = "22.05";
}

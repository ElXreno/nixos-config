{
  config,
  inputs,
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
    inputs.nixos-hardware.nixosModules.common-pc-laptop
    inputs.nixos-hardware.nixosModules.common-pc-laptop-ssd
    inputs.lanzaboote.nixosModules.lanzaboote
    inputs.self.diskoConfigurations.infinity
    inputs.self.nixosRoles.laptop
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.zfs
    inputs.self.nixosProfiles.virtualisation
  ];

  device = {
    isLaptop = true;

    laptop = {
      manufacturer.Honor = true;
      model = "MagicBook 15 2021";
    };

    cpu.amd = true;

    gpu.amd = true;

    network.hasWirelessCard = true;
    network.wirelessCard = "AX200";

    users.alena = true;
  };

  # SecureBoot
  # boot = {
  #   bootspec.enable = true;
  #   loader.systemd-boot.enable = lib.mkForce false;
  #   lanzaboote = {
  #     enable = true;
  #     pkiBundle = "/etc/secureboot";
  #   };
  # };

  boot.extraModprobeConfig = ''
    options iwlwifi amsdu_size=3
    options iwlmvm power_scheme=1
  '';

  hardware.amdgpu = {
    amdvlk.enable = true;
    initrd.enable = true;
    opencl.enable = true;
  };

  programs.nix-ld.enable = true;
  services.bpftune.enable = true;

  home-manager.users.alena = {
    home = {
      inherit (config.system) stateVersion;
      packages = with pkgs; [
        firefox

        telegram-desktop

        # Office and language packs
        libreoffice
        hunspellDicts.ru-ru
      ];
    };
    services.syncthing.enable = true;
  };

  system.stateVersion = "25.05";
}

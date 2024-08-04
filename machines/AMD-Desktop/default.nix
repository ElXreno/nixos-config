{ config, inputs, pkgs, ... }: {
  imports = [
    ./hardware-configuration.nix
    inputs.self.nixosRoles.desktop
    inputs.self.nixosProfiles.tailscale
    inputs.self.nixosProfiles.kde
    inputs.self.nixosProfiles.zfs
  ];

  deviceSpecific.devInfo.legacy = true;

  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="sd[a-z]", ATTRS{queue/rotational}=="1", RUN+="${pkgs.coreutils}/bin/echo 0 > /sys/block/%k/queue/wbt_lat_usec"
  '';

  i18n.defaultLocale = "ru_RU.UTF-8";

  services.xserver.videoDrivers = [ "nvidia" ];
  hardware.nvidia.package =
    config.boot.kernelPackages.nvidiaPackages.legacy_470;

  home-manager.users.alena = {
    home = {
      inherit (config.system) stateVersion;
      packages = with pkgs; [
        firefox-bin

        tdesktop

        # Office and language packs
        libreoffice
        hunspellDicts.ru-ru
      ];
    };
    services.syncthing.enable = true;
  };

  networking.firewall = {
    allowedTCPPorts = [ 34381 ]; # TCP based sync protocol traffic
    allowedUDPPorts = [ 21028 ]; # Syncthing discovery broadcasts
  };

  services.bpftune.enable = true;

  system.stateVersion = "22.05";
}

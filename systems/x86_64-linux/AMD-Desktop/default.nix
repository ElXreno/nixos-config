{
  config,
  namespace,
  pkgs,
  ...
}:
{
  imports = [
    ./hardware-configuration.nix
  ];

  ${namespace} = {
    roles = {
      desktop.enable = true;
    };

    system = {
      boot.legacy.enable = true;

      hardware = {
        cpu.manufacturer = "amd";
        gpu.nvidia = {
          enable = true;
          package = config.boot.kernelPackages.nvidiaPackages.legacy_470;
        };
      };

      fs.zfs = {
        enable = true;
        hostId = "2d73528c";
      };
    };

    desktop-environments.plasma.enable = true;
  };

  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="sd[a-z]", ATTRS{queue/rotational}=="1", RUN+="${pkgs.coreutils}/bin/echo 0 > /sys/block/%k/queue/wbt_lat_usec"
  '';

  i18n.defaultLocale = "ru_RU.UTF-8";

  system.stateVersion = "22.05";
}

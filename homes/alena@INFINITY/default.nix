{ namespace, pkgs, ... }:
{
  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    desktop-environments.plasma.enable = true;

    services.syncthing.enable = true;
  };

  home = {
    packages = with pkgs; [
      telegram-desktop
    ];
  };
}

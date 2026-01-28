{ namespace, pkgs, ... }:
{
  ${namespace} = {
    roles = {
      desktop.enable = true;
    };

    desktop-environments.plasma = {
      enable = true;
      wayland = false;
    };
  };

  home = {
    packages = with pkgs; [
      telegram-desktop

      # Office and language packs
      libreoffice
      hunspellDicts.ru-ru
    ];
  };
}

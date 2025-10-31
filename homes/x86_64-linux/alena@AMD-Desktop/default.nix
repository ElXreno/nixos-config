{ namespace, pkgs, ... }:
{
  ${namespace} = {
    roles = {
      desktop.enable = true;
    };

    desktop-environments.plasma.enable = true;
  };

  home = {
    packages = with pkgs; [
      telegram-desktop

      # Office and language packs
      libreoffice
      hunspellDicts.ru-ru
    ];
  };

  home.stateVersion = "25.05";
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.plasma;
in
{
  options.${namespace}.desktop-environments.plasma = {
    enable = mkEnableOption "managing plasma";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services = {
        xserver.enable = true;
        printing.enable = true;
      };
    };
    services = {
      displayManager.sddm = {
        enable = true;
        wayland.enable = true;
      };
      desktopManager.plasma6 = {
        enable = true;
      };
    };

    environment.plasma6.excludePackages = with pkgs; [ kdePackages.elisa ];

    services.colord.enable = true;

    fonts.packages = with pkgs; [ jetbrains-mono ];

    environment.systemPackages =
      with pkgs;
      with kdePackages;
      [
        # Utilities for Info Center
        clinfo
        mesa-demos
        vulkan-tools

        ark
        gwenview
        kate
        kompare
        kleopatra
        okular
        okteta
        kdiskmark

        # Ark dependency
        unrar

        # Icons
        papirus-icon-theme

        gnome-color-manager
      ];

    programs.dconf.enable = true;
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    optional
    ;
  cfg = config.${namespace}.desktop-environments.plasma;
in
{
  options.${namespace}.desktop-environments.plasma = {
    enable = mkEnableOption "Whether or not to manage plasma.";
    wayland = mkOption {
      type = types.bool;
      default = true;
      description = "Whether to enable wayland.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.services.xserver.enable = true;
    services = {
      displayManager = {
        sddm = {
          enable = true;
          wayland.enable = cfg.wayland;
        };

        defaultSession = mkIf (!cfg.wayland) "plasmax11";
      };
      desktopManager.plasma6 = {
        enable = true;
      };
    };

    environment.plasma6.excludePackages = with pkgs; [ kdePackages.elisa ];

    services.colord.enable = cfg.wayland;

    fonts.packages = with pkgs; [ jetbrains-mono ];

    environment.systemPackages =
      with pkgs;
      with kdePackages;
      [
        # Utilities for Info Center
        clinfo
        glxinfo
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
      ]
      ++ optional cfg.wayland gnome-color-manager;

    programs.dconf.enable = true;
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.hyprland;
in
{
  options.${namespace}.desktop-environments.hyprland = {
    enable = mkEnableOption "Whether or not to manage hyprland.";
  };

  config = mkIf cfg.enable {
    services.displayManager.sddm = {
      enable = true;
      wayland.enable = true;
      package = pkgs.kdePackages.sddm;

      theme = "sddm-astronaut-theme";

      extraPackages = with pkgs; [
        kdePackages.qtmultimedia
        kdePackages.qtsvg
        kdePackages.qtvirtualkeyboard
      ];
    };

    environment.systemPackages =
      let
        sddm-astronaut = pkgs.sddm-astronaut.override {
          embeddedTheme = "pixel_sakura_static";
          themeConfig = {
            Background = toString pkgs.${namespace}.custom-wallpaper;

            FormPosition = "left";

            BackgroundColor = "#141821";
            DimBackgroundColor = "#10131A";
            FormBackgroundColor = "#1B1E25";

            DateTextColor = "#93A4B5";
            TimeTextColor = "#E6EDF3";

            PlaceholderTextColor = "#A9B4BF";

            LoginFieldBackgroundColor = "#d4d9e9ff";
            PasswordFieldBackgroundColor = "#d4d9e9ff";
            LoginFieldTextColor = "#E2B714";
            PasswordFieldTextColor = "#E2B714";

            UserIconColor = "#E2B714";
            PasswordIconColor = "#E2B714";

            SessionButtonTextColor = "#93A4B5";

            HighlightBackgroundColor = "#E2B714";
            HighlightTextColor = "#0E1117";
            HighlightBorderColor = "transparent";

            HoverSessionButtonTextColor = "#F3CC26";

            WarningColor = "#D6A50F";
          };
        };
      in
      [
        sddm-astronaut
      ];

    services = {
      power-profiles-daemon.enable = true;
      udisks2.enable = true;
      colord.enable = true;
    };

    programs.hyprland = {
      enable = true;
      withUWSM = true;
    };

    xdg.portal = {
      enable = true;
      extraPortals = with pkgs; [
        xdg-desktop-portal-hyprland
        xdg-desktop-portal-gtk
        kdePackages.xdg-desktop-portal-kde
      ];

      xdgOpenUsePortal = true;
      config = {
        common.default = [ "kde" ];
        hyprland.default = [
          "hyprland"
          "kde"
          "gtk"
        ];
      };
    };

    environment.sessionVariables = {
      NIXOS_OZONE_WL = "1";
      GDK_BACKEND = "wayland,x11";
      QT_QPA_PLATFORM = "wayland;xcb";
      QT_WAYLAND_DISABLE_WINDOWDECORATION = 1;
    };

    services.udev.extraRules = ''
      SUBSYSTEM=="leds", KERNEL=="platform::micmute", RUN="${pkgs.coreutils}/bin/chmod a+rw /sys/class/leds/platform::micmute/brightness"
    '';

    fonts.packages = with pkgs; [
      jetbrains-mono
      nerd-fonts.jetbrains-mono
    ];
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkEnableOption;
  cfg = config.${namespace}.stylix;
in
{
  options.${namespace}.stylix = {
    enable = mkEnableOption "stylix";
  };

  config = {
    stylix = {
      inherit (cfg) enable;
      enableReleaseChecks = false;
      overlays.enable = false;

      base16Scheme = {
        name = "Dataviz Dark";
        author = "derived from the dataviz reference palette";
        variant = "dark";
        base00 = "1a1a19";
        base01 = "2a2927";
        base02 = "393835";
        base03 = "62615d";
        base04 = "7b7975";
        base05 = "cccac3";
        base06 = "d9d7d0";
        base07 = "ffffff";
        base08 = "ff6366";
        base09 = "ff7c4a";
        base0A = "f5ae46";
        base0B = "45c640";
        base0C = "21cec6";
        base0D = "4f9dfc";
        base0E = "b799ff";
        base0F = "f485ae";
      };
      polarity = "dark";
      image = pkgs.${namespace}.custom-wallpaper;

      fonts.monospace = {
        package = pkgs.nerd-fonts.jetbrains-mono;
        name = "JetBrainsMono Nerd Font";
      };

      icons = {
        enable = true;
        package = pkgs.papirus-icon-theme;
        dark = "Papirus-Dark";
        light = "Papirus-Light";
      };

      targets = {
        qt.standardDialogs = "xdgdesktopportal";
        rofi.enable = false;
      };
    };
  };
}

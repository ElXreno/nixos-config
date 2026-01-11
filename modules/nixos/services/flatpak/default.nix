{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.flatpak;
in
{
  options.${namespace}.services.flatpak = {
    enable = mkEnableOption "Whether or not to manage flatpak.";
  };

  config = mkIf cfg.enable {
    services.flatpak.enable = true;
    environment.systemPackages = with pkgs; [
      flatpak-builder

      appstream
      desktop-file-utils
      ostree-full
      gdk-pixbuf
      librsvg
    ];

    environment.pathsToLink = [
      "share/thumbnailers"
    ];

    environment.sessionVariables = {
      # Use librsvg's gdk-pixbuf loader cache file as it enables gdk-pixbuf to load
      # SVG files (important for icons)
      # Fixes error: .../share/icons/hicolor/scalable/apps/blabla.svg is not a valid icon: Format not recognized
      GDK_PIXBUF_MODULE_FILE = "${pkgs.librsvg}/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache";
    };
  };
}

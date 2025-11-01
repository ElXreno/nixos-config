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
      appstream
      flatpak-builder
      desktop-file-utils
    ];
  };
}

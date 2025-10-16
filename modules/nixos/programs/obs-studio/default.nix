{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.obs-studio;
in
{
  options.${namespace}.programs.obs-studio = {
    enable = mkEnableOption "Whether or not to manage obs-studio.";
  };

  config = mkIf cfg.enable {
    programs.obs-studio = {
      enable = true;
      enableVirtualCamera = true;
      plugins = with pkgs.obs-studio-plugins; [
        wlrobs
        obs-advanced-masks
        obs-pipewire-audio-capture
      ];
    };
  };
}

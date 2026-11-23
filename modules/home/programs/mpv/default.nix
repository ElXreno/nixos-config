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
    optionalAttrs
    ;
  cfg = config.${namespace}.programs.mpv;
in
{
  options.${namespace}.programs.mpv = {
    enable = mkEnableOption "managing MPV";
    vulkanDevice = mkOption {
      type = with types; nullOr str;
      default = null;
    };
  };

  config = mkIf cfg.enable {
    xdg.mimeApps.defaultApplicationPackages = [ config.programs.mpv.finalPackage ];

    programs.mpv = {
      enable = true;
      scripts = with pkgs.mpvScripts; [
        mpris
        sponsorblock
        quality-menu
      ];
      config = {
        ytdl-format = "bestvideo[height<=1080]+(bestaudio[acodec=opus]/bestaudio)/best[height<=1080]";
        hwdec = "vulkan,vaapi";
        hwdec-codecs = "all";
        vo = "gpu-next";
        gpu-api = "vulkan";
        save-position-on-quit = "yes";
        hr-seek = "yes";
        demuxer-max-bytes = "128M";
        demuxer-max-back-bytes = "32M";

        video-sync = "display-resample";
      }
      // optionalAttrs (cfg.vulkanDevice != null) {
        vulkan-device = cfg.vulkanDevice;
      };

      bindings = {
        "F" = "script-binding quality_menu/video_formats_toggle";
        "Alt+f" = "script-binding quality_menu/audio_formats_toggle";
      };
    };
  };
}

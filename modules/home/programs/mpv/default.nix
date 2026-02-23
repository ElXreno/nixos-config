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

  package = pkgs.mpv.override {
    scripts = with pkgs.mpvScripts; [
      mpris
      sponsorblock
      quality-menu
    ];
    youtubeSupport = true;
  };
in
{
  options.${namespace}.programs.mpv = {
    enable = mkEnableOption "Whether or not to manage MPV.";
    vulkanDevice = mkOption {
      type = with types; nullOr str;
      default = null;
    };
  };

  config = mkIf cfg.enable {
    programs.mpv = {
      inherit package;

      enable = true;
      config = {
        ytdl-format = "bestvideo[height<=1080]+(bestaudio[acodec=opus]/bestaudio)/best[height<=1080]";
        hwdec = "vulkan";
        hwdec-codecs = "all";
        vo = "gpu-next";
        gpu-api = "vulkan";
        save-position-on-quit = "yes";
        hr-seek = "yes";
        demuxer-max-bytes = "128M";
        demuxer-max-back-bytes = "32M";

        video-sync = "display-resample";

        target-colorspace-hint = "no";
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

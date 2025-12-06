{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.mpv;
in
{
  options.${namespace}.programs.mpv = {
    enable = mkEnableOption "Whether or not to manage MPV.";
  };

  config = mkIf cfg.enable {
    programs.mpv = {
      enable = true;
      config = {
        ytdl-format = "bestvideo[height<=1080]+(bestaudio[acodec=opus]/bestaudio)/best[height<=1080]";
        hwdec = "nvdec-copy,auto";
        hwdec-codecs = "all";
        vo = "gpu-next";
        gpu-api = "vulkan";
        save-position-on-quit = "yes";
        hr-seek = "yes";
        demuxer-max-bytes = "128M";
        demuxer-max-back-bytes = "32M";

        video-sync = "display-resample";
      };

      bindings = {
        "Ctrl+l" = "vf toggle vapoursynth=~~/motioninterpolation.py:buffered-frames=4:concurrent-frames=3";
        "F" = "script-binding quality_menu/video_formats_toggle";
        "Alt+f" = "script-binding quality_menu/audio_formats_toggle";
      };
    };

    home.file.".config/mpv/motioninterpolation.py".source = ./motioninterpolation.py;
  };
}

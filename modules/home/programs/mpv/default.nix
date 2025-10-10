{
  config,
  namespace,
  lib,
  specialArgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.mpv;

  withNvidia = specialArgs.withNvidia or false; # TODO: Doesn't work
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
        hwdec = if withNvidia then "nvdec-copy" else "auto";
        hwdec-codecs = "all";
        gpu-context = "wayland";
        vo = "gpu-next";
        save-position-on-quit = "yes";
        hr-seek = "yes";
        demuxer-max-bytes = "128M";
        demuxer-max-back-bytes = "32M";
      };
    };
  };
}

{
  home-manager.users.elxreno.programs.mpv = {
    enable = true;
    config = {
      ytdl-format = "bestvideo[height<=1080]+(bestaudio[acodec=opus]/bestaudio)/best[height<=1080]";
      hwdec = "auto";
      vo = "gpu";
      save-position-on-quit = "yes";
      hr-seek = "yes";
      demuxer-max-bytes = "300M";
      demuxer-max-back-bytes = "100M";
    };
  };
}

{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.fish;
in
{
  options.${namespace}.programs.fish = {
    enable = mkEnableOption "Whether or not to manage fish.";
  };

  config = mkIf cfg.enable {
    programs.fish = {
      enable = true;
      shellAliases = {
        nrs = "nixos-rebuild switch -v --sudo";
        nrb = "nixos-rebuild boot -v --sudo";
        nrt = "nixos-rebuild test -v --sudo";
        ssr = "sudo systemctl restart";
        sss = "sudo systemctl status";
        sst = "sudo systemctl stop";
        ytdl = "yt-dlp -f '(bestvideo[vcodec=vp9][height<=1080]/bestvideo[height<=1080])+(bestaudio[acodec=opus]/bestaudio)'";
        ytf = "yt-dlp -F";
      };
    };
  };
}

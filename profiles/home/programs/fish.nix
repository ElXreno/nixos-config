{ pkgs, ... }: {
  programs.fish.enable = true;
  home-manager.users.elxreno.programs.fish = {
    enable = true;
    shellAliases = {
      nrs = "nixos-rebuild switch -v --use-remote-sudo";
      nrb = "nixos-rebuild boot -v --use-remote-sudo";
      nrt = "nixos-rebuild test -v --use-remote-sudo";
      ssr = "sudo systemctl restart";
      sss = "sudo systemctl status";
      sst = "sudo systemctl stop";
      ytdl =
        "yt-dlp -f '(bestvideo[vcodec=vp9][height<=1080]/bestvideo[height<=1080])+(bestaudio[acodec=opus]/bestaudio)'";
      ytf = "yt-dlp -F";
    };
    plugins = [{
      name = "bass";
      src = pkgs.fetchFromGitHub {
        owner = "edc";
        repo = "bass";
        rev = "2fd3d2157d5271ca3575b13daec975ca4c10577a";
        sha256 = "sha256-fl4/Pgtkojk5AE52wpGDnuLajQxHoVqyphE90IIPYFU=";
      };
    }];
  };
}

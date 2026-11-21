{
  config,
  osConfig,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption optional;
  cfg = config.${namespace}.roles.workstation;
  home = config.home.homeDirectory;
in
{
  options.${namespace}.roles.workstation = {
    enable = mkEnableOption "workstation configuration" // {
      default = osConfig.${namespace}.roles.workstation.enable;
    };
  };

  config = mkIf cfg.enable {
    home = {
      packages = with pkgs; [
        keepassxc
        thunderbird
        wgcf
        ffmpeg-full
        yt-dlp
        gh
        glab
        discord
        betterdiscordctl
        kubectl
        freelens-bin
        obsidian
        sony-headphones-client
      ];

      sessionVariables = {
        DOTNET_CLI_TELEMETRY_OPTOUT = 1;
        RUFF_CACHE_DIR = "${config.xdg.cacheHome}/ruff";
      };
    };

    programs.chromium.enable = true;

    programs.fish.shellAliases = {
      ytdl = "yt-dlp -f '(bestvideo[vcodec=vp9][height<=1080]/bestvideo[height<=1080])+(bestaudio[acodec=opus]/bestaudio)'";
      ytf = "yt-dlp -F";
    };

    xdg.mimeApps.associations.removed = {
      "application/zip" = "org.prismlauncher.PrismLauncher.desktop";
      "application/x-modrinth-modpack+zip" = "org.prismlauncher.PrismLauncher.desktop";
    };

    ${namespace} = {
      common-packages.enable = true;

      places = [
        "file://${home}/Downloads"
        "file://${home}/Sync"
        "file://${home}/projects Projects"
        "file://${home}/projects/repos/github.com/ElXreno GH/ElXreno"
      ]
      ++ optional (osConfig.fileSystems ? "/mnt/datalake") "file:///mnt/datalake Datalake"
      ++ [ "file:///tmp" ];

      programs = {
        claude-code.enable = true;
        darktable.enable = true;
        go.enable = true;
        herdr.enable = true;
        spicetify.enable = true;
        ssh.enable = true;
        telegram-desktop.enable = true;
        zed-editor.enable = true;
      };

      services = {
        github-watch.enable = true;
        ssh-agent.enable = true;
        syncthing.enable = true;
      };
    };
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.common-packages;
in
{
  options.${namespace}.common-packages = {
    enable = mkEnableOption "Whether or not to provision common packages.";
  };

  config = mkIf cfg.enable {
    home = {
      packages =
        with pkgs;
        lib.mkMerge [
          [
            # CLI Stuff
            age
            aria2
            attic-client
            binutils
            binwalk
            bmon
            compsize
            dua
            fd
            file
            inetutils
            iotop
            jq
            mtr
            pciutils
            pigz
            restic
            ripgrep
            screen
            smartmontools
            sops
            tree
            unzip
            usbutils
            wget

            # Nix stuff
            deadnix
            nix-tree
            nixfmt
            statix
            nix-output-monitor
            nurl
          ]
          (lib.mkIf (!config.${namespace}.roles.server.enable) [
            # Messengers
            telegram-desktop

            # Etc
            keepassxc
            qbittorrent
            thunderbird
          ])
          (lib.mkIf config.${namespace}.roles.laptop.enable (
            [
              # CLI Stuff
              deploy-rs
              wgcf
              ffmpeg-full
              yt-dlp

              # Messengers
              discord
              betterdiscordctl

              # Games
              taterclient-ddnet
              gamemode
              prismlauncher
              xclicker
              lutris

              # MEGA
              megasync

              # Office and language packs
              libreoffice
              hunspellDicts.ru-ru

              # Photos
              # darktable
              rawtherapee
              # upscaler

              # Libvirtd manager
              virt-manager

              # Music
              spotify

              # K8s
              kubectl
              lens
              freelens-bin

              # Self-organization
              obsidian
              super-productivity

              ## etc
              sony-headphones-client
              # chromium
              hashcat
            ]
            ++ (with pkgs.${namespace}; [
              # K8s
              headlamp

              qcsuper
              signalcat
            ])
          ))
        ];
    };

    programs.chromium.enable = true;

    xdg.mimeApps = {
      defaultApplications = {
        "x-scheme-handler/lens" = "lens-desktop.desktop";
        "video/x-matroska" = "mpv.desktop";
        "video/mpeg" = "mpv.desktop";
      };

      associations.removed = {
        "application/zip" = "org.prismlauncher.PrismLauncher.desktop";
        "application/x-modrinth-modpack+zip" = "org.prismlauncher.PrismLauncher.desktop";
      };
    };
  };
}

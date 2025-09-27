{
  config,
  lib,
  pkgs,
  ...
}:
{
  environment.systemPackages = lib.mkIf (!config.deviceSpecific.isServer) (
    with pkgs;
    [
      gparted
      e2fsprogs
    ]
  );

  home-manager.users.elxreno = {
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
            config.boot.kernelPackages.cpupower
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
            nixfmt-rfc-style
            statix
            nix-output-monitor
            nurl
          ]
          (lib.mkIf (!config.deviceSpecific.isServer) [
            # Messengers
            telegram-desktop

            # Etc
            keepassxc
            qbittorrent
            thunderbird
          ])
          (lib.mkIf config.deviceSpecific.isLaptop [
            # CLI Stuff
            deploy-rs
            wgcf
            ffmpeg-full
            yt-dlp

            # Messengers
            discord

            # Games
            taterclient-ddnet
            gamemode
            prismlauncher
            xclicker

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
            headlamp

            # Self-organization
            obsidian

            ## etc
            sony-headphones-client
            chromium
          ])
        ];
    };

    xdg.mimeApps = {
      defaultApplications = {
        "x-scheme-handler/lens" = lib.mkIf config.deviceSpecific.isLaptop "lens-desktop.desktop";
        "video/x-matroska" = lib.mkIf config.deviceSpecific.isLaptop "mpv.desktop";
        "video/mpeg" = lib.mkIf config.deviceSpecific.isLaptop "mpv.desktop";
      };

      associations.removed = lib.mkIf config.deviceSpecific.isLaptop {
        "application/zip" = "org.prismlauncher.PrismLauncher.desktop";
        "application/x-modrinth-modpack+zip" = "org.prismlauncher.PrismLauncher.desktop";
      };
    };
  };
}

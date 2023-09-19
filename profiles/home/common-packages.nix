{ config, lib, pkgs, ... }:
let
  megasync_autostart = pkgs.makeAutostartItem { name = "megasync"; package = pkgs.megasync; };
in
{
  environment.systemPackages = lib.mkIf (!config.deviceSpecific.isServer) (with pkgs; [
    gparted
  ]);

  home-manager.users.elxreno = {
    home = {
      packages = with pkgs; lib.mkMerge [
        [
          # CLI Stuff
          age
          aria2
          binutils
          binwalk
          bmon
          brotli
          cachix
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
          wireguard-tools

          # Nix stuff
          deadnix
          nix-prefetch-github
          nix-tree
          nixpkgs-fmt
          nixpkgs-review
          statix
        ]
        (lib.mkIf (!config.deviceSpecific.isServer) [
          # Messengers
          tdesktop

          # Etc
          keepassxc
          qbittorrent
          thunderbird
        ])
        (lib.mkIf (config.deviceSpecific.isLaptop)
          [
            # CLI Stuff
            deploy-rs
            elfshaker
            wgcf
            ffmpeg-full
            gitRepo
            yt-dlp

            # Messengers
            discord-canary
            element-desktop
            slack

            # GraalVM
            graalvm17-ee

            # Games
            bottles
            ddnet
            gamemode
            mangohud
            prismlauncher
            xclicker

            # MEGA
            megasync
            megasync_autostart

            # DB
            pgmodeler

            # Dev
            gitkraken
            postman
            jetbrains.pycharm-professional
            jetbrains.clion

            # Office and language packs
            libreoffice
            hunspellDicts.ru-ru

            # Photos
            darktable
            rawtherapee
            upscaler

            # Libvirtd manager
            virt-manager

            # Music
            strawberry

            # K8s
            kubectl
            # openlens lacks some features like `shell in the pod`
            lens

            # Unrelated work
            lsfusion-client

            ## etc
            av1an
            remmina
          ])
      ];
    };

    xdg.mimeApps = {
      defaultApplications = {
        "x-scheme-handler/element" = lib.mkIf (!config.deviceSpecific.isServer) "element-desktop.desktop";
        "x-scheme-handler/gitkraken" = lib.mkIf config.deviceSpecific.isLaptop "gitkraken.desktop";
        "x-scheme-handler/lens" = lib.mkIf config.deviceSpecific.isLaptop "lens-desktop.desktop";
        "x-scheme-handler/postman" = lib.mkIf config.deviceSpecific.isLaptop "Postman.desktop";
        "x-scheme-handler/slack" = lib.mkIf config.deviceSpecific.isLaptop "slack.desktop";
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

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
          bintools-unwrapped
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
          element-desktop
          tdesktop

          keepassxc
          qbittorrent
          thunderbird
        ])
        (lib.mkIf (config.deviceSpecific.isLaptop)
          [
            # CLI Stuff
            acpi
            deploy-rs
            elfshaker
            ngrok
            wgcf
            ffmpeg-full
            gitRepo
            yt-dlp

            # GraalVM
            graalvm17-ee

            # flatpak stuff
            flatpak-builder
            debugedit-unstable

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
            jetbrains.clion
            jetbrains.datagrip
            postman

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

            ## etc
            remmina
          ])
      ];
    };

    xdg.mimeApps = {
      defaultApplications = {
        "x-scheme-handler/element" = lib.mkIf (!config.deviceSpecific.isServer) "element-desktop.desktop";
        "x-scheme-handler/gitkraken" = lib.mkIf config.deviceSpecific.isLaptop "gitkraken.desktop";
        "x-scheme-handler/lens" = lib.mkIf config.deviceSpecific.isLaptop "lens.desktop";
        "x-scheme-handler/postman" = lib.mkIf config.deviceSpecific.isLaptop "Postman.desktop";
      };

      associations.removed = lib.mkIf config.deviceSpecific.isLaptop {
        "application/zip" = "org.prismlauncher.PrismLauncher.desktop";
        "application/x-modrinth-modpack+zip" = "org.prismlauncher.PrismLauncher.desktop";
      };
    };
  };
}

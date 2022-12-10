{ config, inputs, lib, pkgs, ... }:
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
          ffmpeg-full
          ffsend
          file
          gitRepo
          inetutils
          iotop
          jq
          mtr
          nmap
          pciutils
          pigz
          restic
          ripgrep
          screen
          smartmontools
          sops
          streamlink
          tree
          unzip
          usbutils
          wget
          yt-dlp

          # Nix stuff
          nix-prefetch-github
          nix-tree
          nixpkgs-fmt
          nixpkgs-review

          # For some python scripts
          python3
        ]
        (lib.mkIf (!config.deviceSpecific.isServer) [
          # Messengers
          element-desktop
          tdesktop

          # Office and language packs
          libreoffice
          hunspellDicts.ru-ru

          keepassxc
          # libnotify
          qbittorrent
          thunderbird
        ])
        (lib.mkIf (config.deviceSpecific.isLaptop || config.device == "nixos-iso")
          (with inputs.nix-alien.packages.${pkgs.stdenv.hostPlatform.system}; with inputs.nixpkgs-update.packages.${pkgs.stdenv.hostPlatform.system}; [
            # CLI Stuff
            acpi
            apktool
            aircrack-ng
            mdk4
            #ngrok
            deploy-rs
            elfshaker
            # diesel-cli
            # cassowary
            wgcf
            statix

            # universal-android-debloater

            # flatpak stuff
            flatpak-builder
            debugedit-unstable

            # Games
            ddnet

            # Messengers
            # slack

            # MEGA
            megasync
            megasync_autostart

            # IDEs editors and other dev stuff

            # go
            # vscode
            # dotnet-sdk_6
            # dotnetCorePackages.runtime_6_0
            # dotnetCorePackages.aspnetcore_6_0
            # mono6
            # kotlin
            # clang-tools

            pgmodeler

            ## Android stuff
            # androidStudioPackages.beta
            # flutter

            # Photos
            digikam
            darktable
            rawtherapee

            ## etc
            gitkraken
            rustup
            # clang
            #notion-repackaged

            # Libvirtd manager
            virt-manager

            # Minecraft
            prismlauncher
            # adoptopenjdk-hotspot-bin-16

            wireguard-tools

            strawberry

            # UEFI stuff
            # ifr-extractor
            # uefitool

            # For kate
            # TODO: Integrate to kate
            # btw this is a slow shit
            # nodePackages.bash-language-server

            nix-casync
            remmina

            # nix-alien
            nix-alien
            nix-index-update

            nixpkgs-update
          ]))
      ];
    };

    xdg.mimeApps.defaultApplications = {
      "x-scheme-handler/element" = lib.mkIf (!config.deviceSpecific.isServer) "element-desktop.desktop";
      "x-scheme-handler/gitkraken" = lib.mkIf config.deviceSpecific.isLaptop "gitkraken.desktop";
    };
  };
}

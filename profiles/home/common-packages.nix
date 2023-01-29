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
          file
          gitRepo
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
          streamlink
          tree
          unzip
          usbutils
          wget
          wireguard-tools
          yt-dlp

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
        (lib.mkIf (config.deviceSpecific.isLaptop || config.device == "nixos-iso")
          [
            # CLI Stuff
            acpi
            deploy-rs
            elfshaker
            ngrok
            wgcf

            # flatpak stuff
            flatpak-builder
            debugedit-unstable

            # Games
            ddnet
            prismlauncher
            gamemode
            bottles
            lutris
            xclicker

            # MEGA
            megasync
            megasync_autostart

            # DB
            pgmodeler

            # Dev
            gitkraken

            # Photos
            darktable
            rawtherapee
            upscaler

            # Libvirtd manager
            virt-manager

            # Music
            strawberry

            ## etc
            remmina
          ])
      ];
    };

    xdg.mimeApps.defaultApplications = {
      "x-scheme-handler/element" = lib.mkIf (!config.deviceSpecific.isServer) "element-desktop.desktop";
      "x-scheme-handler/gitkraken" = lib.mkIf config.deviceSpecific.isLaptop "gitkraken.desktop";
    };
  };
}

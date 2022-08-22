{ config, inputs, pkgs, lib, ... }:
let
  optimizeForThisHost = pkg:
    pkg.overrideAttrs (attrs: {
      NIX_CFLAGS_COMPILE = (attrs.NIX_CFLAGS_COMPILE or "") + " -march=znver2 -mtune=znver2 -Ofast -fPIC -ffat-lto-objects -flto=auto -funroll-loops -fomit-frame-pointer ";
      RUSTFLAGS = (attrs.RUSTFLAGS or "") + " -C target-cpu=native";
    });
  optimizeForThisHostStdenv = pkg:
    pkg.override {
      stdenv = pkgs.stdenvAdapters.addAttrsToDerivation
        {
          NIX_CFLAGS_COMPILE = "-march=znver2 -mtune=znver2 -Ofast -fPIC -ffat-lto-objects -flto=auto -funroll-loops -fomit-frame-pointer";
          RUSTFLAGS = "-C target-cpu=native";
        }
        pkgs.stdenv;
    };
in
{
  nixpkgs = {
    config = {
      allowUnfreePredicate = pkg: builtins.elem (lib.getName pkg) [
        "gitkraken"
        "megasync"
        "nvidia-settings"
        "nvidia-x11"
        "unrar"
        "vscode"

        # IDE
        "clion"
        "rider"
      ];
      permittedInsecurePackages = [
        "electron-11.5.0"
      ];
    };
    overlays = with inputs; [
      rust-overlay.overlays.default
      (self: super:
        {
          # stdenv = super.stdenvAdapters.addAttrsToDerivation
          #   {
          #     NIX_CFLAGS_COMPILE = "-march=native -O3 -fPIC -ffat-lto-objects -flto=auto";
          #   }
          #   super.stdenv;
          # Hack for rustc
          # rustup = (super.rustup.overrideAttrs (oldAttrs: rec {
          #   propagatedBuildInputs = [ super.clang ];
          # }));

          # linux-firmware = (super.linux-firmware.overrideAttrs (oldAttrs: rec {
          #   version = "main";
          #   outputHash = "sha256-npcSuoAVanRja/8jagsveVjLjjM9p2xKRj5kdWmxEUs=";
          #   src = super.fetchgit {
          #     url = "https://git.kernel.org/pub/scm/linux/kernel/git/firmware/linux-firmware.git";
          #     rev = "eb8ea1b46893c42edbd516f971a93b4d097730ab";
          #     sha256 = "sha256-r4n0beZ/QqnFmrm48kpsfSbnXWWxVDggsBsbXCrueQE=";
          #   };
          # }));

          # ddnet = optimizeForThisHost (super.callPackage ../modules/ddnet.nix { });
          # notion-app = super.callPackage ../modules/notion-app.nix { };
          # ipfs = (super.ipfs.overrideAttrs (old:
          #   let
          #     version = "0.10.0-rc1";
          #     src = super.fetchurl {
          #       url = "https://github.com/ipfs/go-ipfs/releases/download/v${version}/go-ipfs-source.tar.gz";
          #       sha256 = "sha256-p9VUAeBuuOOg6vJMLl2t+vajNQOHRtjn+Bv6kpMAEE8=";
          #     };
          #   in
          #   rec {
          #     name = "ipfs-${version}";
          #     inherit src;
          #     inherit (super.buildGoModule {
          #       inherit name src;
          #       vendorSha256 = null;
          #     }) go-modules;
          #   })
          # );
          # papermc =
          #   (super.papermc.overrideAttrs
          #     (old:
          #       let
          #         mcVersion = "1.17.1";
          #         buildNum = "349";
          #         jar = super.fetchurl {
          #           url = "https://papermc.io/api/v1/paper/${mcVersion}/${buildNum}/download";
          #           sha256 = "sha256:0d7q6v5w872phcgkha7j5sxniqq9wqbh1jxdvyvy6d2jl74g1gzw";
          #         };
          #       in
          #       super.stdenv.mkDerivation {
          #         version = "${mcVersion}r${buildNum}";
          #       }
          #     )
          #   );
          bluez5-experimental = super.bluez5-experimental.overrideAttrs (old: {
            patches = (old.patches or [ ]) == [
              (super.fetchpatch {
                url = "https://patchwork.kernel.org/project/bluetooth/patch/20210514211304.17237-1-luiz.dentz@gmail.com/raw/";
                sha256 = "sha256-SnERSCMo7KPgZV4yC1eYwDBg+iPxoB0Ve7l2VX97KrA=";
              })
            ];
          });
          # ark = optimizeForThisHost (super.ark);
          # papermc = super.papermc.override { jre = pkgs.adoptopenjdk-jre-hotspot-bin-16; };
          # jdkNative = optimizeForThisHost super.jdk;
          # polymc = (optimizeForThisHost (super.polymc)).override { jdk = self.jdkNative; };
          wg-bond = inputs.wg-bond.defaultPackage.${super.system};
          deploy-rs = inputs.deploy-rs.defaultPackage.${super.system};
          # smart-home-server = inputs.smart-home-server.defaultPackage.${super.system};
          # multimc = super.multimc.override { jdk = pkgs.adoptopenjdk-hotspot-bin-16; };
          # crowdsec = super.callPackage ../modules/crowdsec { };
          ifr-extractor = super.callPackage ../modules/ifr-extractor.nix { };
          av1an = super.callPackage ../modules/av1an.nix { };
          elfshaker = super.callPackage ../modules/elfshaker.nix { };
          nix-casync = super.callPackage ../modules/nix-casync.nix { };
          cassowary = super.callPackage ../modules/cassowary.nix { };
          universal-android-debloater = super.callPackage ../modules/universal-android-debloater.nix { };
          bees = optimizeForThisHostStdenv super.bees;
        })
    ];
  };
}

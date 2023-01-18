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
        "ngrok"
        "nvidia-settings"
        "nvidia-x11"
        "steam-original"
        "steam-run"
        "steam"
        "unrar"
        "vscode"

        # IDE
        "android-studio-canary"
        "clion"
        "idea-ultimate"
        "phpstorm"
        "pycharm-professional"
        "rider"

        "vscode-extension-github-copilot"
      ];
    };
    overlays = with inputs; [
      rust-overlay.overlays.default
      (self: super:
        {
          bluez5-experimental = super.bluez5-experimental.overrideAttrs (old: {
            patches = (old.patches or [ ]) == [
              (super.fetchpatch {
                url = "https://patchwork.kernel.org/project/bluetooth/patch/20210514211304.17237-1-luiz.dentz@gmail.com/raw/";
                sha256 = "sha256-SnERSCMo7KPgZV4yC1eYwDBg+iPxoB0Ve7l2VX97KrA=";
              })
            ];
          });

          tlp = super.tlp.override { inherit (config.boot.kernelPackages) x86_energy_perf_policy; };

          deploy-rs = inputs.deploy-rs.defaultPackage.${super.system};

          prismlauncher = super.prismlauncher.override { jdk17 = pkgs.graalvm17-ce; jdks = with pkgs; [ graalvm17-ce ]; };

          av1an = super.callPackage ../modules/av1an.nix { };
          cassowary = super.callPackage ../modules/cassowary.nix { };
          elfshaker = super.callPackage ../modules/elfshaker.nix { };
          ifr-extractor = super.callPackage ../modules/ifr-extractor.nix { };

          fun-quiz-server = inputs.fun-quiz-server.packages.${super.system}.default;
        })
    ];
  };
}

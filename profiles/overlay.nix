{ config, inputs, lib, ... }:
let
  optimizedPkgs = import inputs.nixpkgs {
    localSystem = {
      gcc = {
        arch = "znver2";
        tune = "znver2";
      };
      inherit (config.nixpkgs) system;
    };

    overlays = [
      (_self: super: {
        python3 = super.python3.override {
          packageOverrides = python-self: python-super: {
            click = python-super.click.overrideAttrs (old: {
              disabledTests = (old.disabledTests or [ ]) ++ [
                "test_file_surrogates" # Invalid or incomplete multibyte or wide character
              ];
            });
          };
        };

        jdk17 = super.jdk17.overrideAttrs (old: {
          configureFlags =
            let cflags = "-Ofast";
            in (old.configureFlags or [ ]) ++ [
              "--with-jvm-variants=server"
              "--with-jvm-features=link-time-opt,zgc"
              "--with-extra-cflags=${cflags}"
              "--with-extra-cxxflags=${cflags}"
            ];
        });
      })
    ];
  };
in
{
  imports = [
    inputs.nur-xddxdd.nixosModules.setupOverlay
  ];

  nixpkgs = {
    config = {
      allowUnfreePredicate = pkg: builtins.elem (lib.getName pkg) [
        "gitkraken"
        "megasync"
        "ngrok"
        "nvidia-settings"
        "nvidia-x11"
        "slack"
        "discord-canary"
        "steam-original"
        "steam-run"
        "steam"
        "svp"
        "unrar"
        "vscode"
        "lens-desktop"

        # IDE
        "android-studio-canary"
        "clion"
        "idea-ultimate"
        "lens"
        "phpstorm"
        "postman"
        "pycharm-professional"
        "rider"

        "vscode-extension-github-copilot"
      ];
      nvidia.acceptLicense = true;
    };
    overlays = with inputs; [
      rust-overlay.overlays.default
      (_self: super:
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

          prismlauncher = super.prismlauncher.override { jdk17 = optimizedPkgs.jdk17; jdks = with optimizedPkgs; [ jdk17 graalvm-ce ]; };

          av1an = super.callPackage ../modules/av1an.nix { };
          cassowary = super.callPackage ../modules/cassowary.nix { };
          elfshaker = super.callPackage ../modules/elfshaker.nix { };
          ifr-extractor = super.callPackage ../modules/ifr-extractor.nix { };
          lsfusion-client = super.callPackage ../modules/lsfusion-client.nix { };
        })
    ];
  };
}

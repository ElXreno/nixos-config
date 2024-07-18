{ config, inputs, pkgs, lib, ... }: {
  imports = [ inputs.nur-xddxdd.nixosModules.setupOverlay ];

  nixpkgs = {
    config = {
      allowUnfreePredicate = pkg:
        builtins.elem (lib.getName pkg) [
          "gitkraken"
          "megasync"
          "ngrok"
          "nvidia-settings"
          "nvidia-x11"
          "slack"
          "discord"
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
          "pycharm-professional"
          "rider"

          "vscode-extension-github-copilot"
        ];
      nvidia.acceptLicense = true;

      permittedInsecurePackages = [
        "freeimage-unstable-2021-11-01" # for megasync
      ];
    };

    overlays = with inputs; [
      rust-overlay.overlays.default
      (_self: super: {
        bluez5-experimental = super.bluez5-experimental.overrideAttrs (old: {
          patches = (old.patches or [ ]) == [
            (super.fetchpatch {
              url =
                "https://patchwork.kernel.org/project/bluetooth/patch/20210514211304.17237-1-luiz.dentz@gmail.com/raw/";
              sha256 = "sha256-SnERSCMo7KPgZV4yC1eYwDBg+iPxoB0Ve7l2VX97KrA=";
            })
          ];
        });

        tlp = super.tlp.override {
          inherit (config.boot.kernelPackages) x86_energy_perf_policy;
        };

        deploy-rs = inputs.deploy-rs.defaultPackage.${super.system};
        teledump = inputs.teledump.packages.${super.system}.default;
        simple-reply-bot =
          inputs.simple-reply-bot.packages.${super.system}.default;

        prismlauncher = super.prismlauncher.override {
          jdk17 = super.jdk17;
          jdks = with super; [ jdk17 graalvm-ce ];
        };

        av1an = super.callPackage ../modules/av1an.nix { };
        cassowary = super.callPackage ../modules/cassowary.nix { };
        elfshaker = inputs.elfshaker.packages.${super.system}.default;
        ifr-extractor = super.callPackage ../modules/ifr-extractor.nix { };
      })
    ];
  };
}

{
  config,
  inputs,
  lib,
  infuse,
  ...
}:
{
  nixpkgs = {
    config = {
      allowUnfreePredicate =
        pkg:
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
          "steam-unwrapped"
          "steam"
          "unrar"
          "vscode"
          "lens-desktop"
          "obsidian"

          # IDE
          "android-studio-canary"
          "clion"
          "idea-ultimate"
          "lens"
          "phpstorm"
          "postman"
          "pycharm-professional"
          "rider"
          "rust-rover"
        ];
      nvidia.acceptLicense = true;
    };

    overlays = with inputs; [
      (_self: super: {
        bluez5-experimental = super.bluez5-experimental.overrideAttrs (old: {
          patches =
            (old.patches or [ ]) == [
              (super.fetchpatch {
                url = "https://patchwork.kernel.org/project/bluetooth/patch/20210514211304.17237-1-luiz.dentz@gmail.com/raw/";
                sha256 = "sha256-SnERSCMo7KPgZV4yC1eYwDBg+iPxoB0Ve7l2VX97KrA=";
              })
            ];
        });

        hydra = super.hydra.overrideAttrs (old: {
          patches = (old.patches or [ ]) ++ [ ../patches/hydra-github-status.patch ];
        });

        nix-lazy-tree-v2 =
          infuse
            (inputs.nix.packages.${super.system}.default.appendPatches [
              (super.fetchpatch {
                url = "https://github.com/moni-dz/nix-config/raw/aa10dd1fa85a69230bfcec010e3e61d5c7658711/packages/patches/lazy-trees-v2.patch";
                sha256 = "sha256-I3uZ87YBdFYCINwJRJROlnsNCTHGeT60N3WV17SEWYg=";
              })
            ])
            {
              __output.doCheck.__assign = false;
            };

        tlp = super.tlp.override {
          inherit (config.boot.kernelPackages) x86_energy_perf_policy;
        };

        deploy-rs = inputs.deploy-rs.defaultPackage.${super.system};

        prismlauncher = super.prismlauncher.override {
          jdks = with super; [
            jdk17
            graalvm-ce
          ];
        };

        headlamp = super.callPackage inputs.self.nixosModules.headlamp { };
      })
    ];
  };
}

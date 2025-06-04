{
  config,
  inputs,
  lib,
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

          # NVidia with CUDA
          "nvidia-settings"
          "nvidia-x11"
          "cuda_cudart"
          "cuda_cccl"
          "libnpp"
          "libcublas"
          "libcufft"
          "cuda_nvcc"

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
      cudaSupport = config.device == "KURWA";
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

{
  config,
  inputs,
  lib,
  ...
}:
let
  optimizedPkgsZnver4 = import inputs.nixpkgs {
    localSystem = {
      inherit (config.nixpkgs) system;
      gcc.arch = "znver4";
      gcc.tune = "znver4";
    };
  };
in
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
          "cuda-merged"
          "cuda_cuobjdump"
          "cuda_gdb"
          "cuda_nvdisasm"
          "cuda_nvprune"
          "cuda_cupti"
          "cuda_cuxxfilt"
          "cuda_nvml_dev"
          "cuda_nvrtc"
          "cuda_nvtx"
          "cuda_profiler_api"
          "cuda_sanitizer_api"
          "libcurand"
          "libcusolver"
          "libnvjitlink"
          "libcusparse"

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
          jdks =
            with super;
            let
              myjre8 =
                if (lib.elem "gccarch-znver4" config.nix.settings.system-features) then
                  optimizedPkgsZnver4.jre8
                else
                  jre8;
            in
            [
            jdk17
            graalvm-ce
              myjre8
          ];
        };

        headlamp = super.callPackage inputs.self.nixosModules.headlamp { };
      })
    ];
  };
}

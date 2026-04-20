{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkForce
    kernel
    mkMerge
    ;
  cfg = config.${namespace}.system.hardware.asus.fa507uv;

  patchesSrc = pkgs.fetchFromGitHub {
    owner = "CachyOS";
    repo = "kernel-patches";
    rev = "b5e029226df5cc30c103651072d49a7af2878202";
    hash = "sha256-b9Hc0sTxjEzDbphzS9yQqxVha/7bsPIs2cQQQvaG45E=";
  };

  majorMinor = lib.versions.majorMinor config.${namespace}.system.boot.kernel.packages.kernel.version;

  overrideMesa =
    mesa:
    (mesa.override {
      galliumDrivers = [
        "llvmpipe"
        "radeonsi"
        "virgl"
        "zink"
      ];
      vulkanDrivers = [
        "amd"
        "virtio"
      ];
    }).overrideAttrs
      (oldAttrs: {
        mesonFlags = oldAttrs.mesonFlags ++ [
          (pkgs.lib.mesonOption "c_args" "-march=znver4")
          (pkgs.lib.mesonOption "cpp_args" "-march=znver4")
          (pkgs.lib.mesonOption "optimization" "3")
        ];

        outputs = lib.filter (out: out != "spirv2dxil") oldAttrs.outputs;
      });
in
{
  options.${namespace}.system.hardware.asus.fa507uv = {
    enable = mkEnableOption "Whether or not to manage ASUS FA507UV stuff.";
  };

  config = mkIf cfg.enable (mkMerge [
    {
      ${namespace}.system.boot.kernel.packages = pkgs.linuxPackages_xanmod_edge;
      boot = {
        kernelPatches = [
          {
            name = "clang-polly";
            patch = "${patchesSrc}/${majorMinor}/misc/0001-clang-polly.patch";
          }
          {
            name = "sched-bore";
            patch = pkgs.runCommand "0001-bore-xanmod-${majorMinor}.patch" { } ''
              substitute ${patchesSrc}/${majorMinor}/sched/0001-bore.patch $out \
                --replace-fail \
                " unsigned int sysctl_sched_tunable_scaling = SCHED_TUNABLESCALING_LOG;" \
                " unsigned int sysctl_sched_tunable_scaling = SCHED_TUNABLESCALING_NONE;"
            '';
          }
          {
            name = "asus-armoury-crate-fa507uv";
            patch = ./kernel-patches/0001-platform-x86-asus-armoury-Add-tunings-for-FA507UV-bo.patch;
          }
          {
            name = "iwlwifi-lar_disable";
            patch = ./kernel-patches/iwlwifi-lar_disable.patch;
          }
          {
            name = "fa507uv-tunables";
            patch = null;
            structuredExtraConfig = with kernel; {
              X86_64_VERSION = mkForce unset;
              MZEN4 = yes;

              ASUS_ARMOURY = module;

              NTSYNC = module;

              DEBUG_LIST = mkForce no;
            };
          }
        ];

        initrd.prepend = [
          # Fix D3cold power state loop (D0 -> D3cold -> D0), thanks ASUS
          # For reference: Remove Notify (\_SB.NPCF, 0xC0) in the `_OFF` method of \_SB.PCI0.GPP0 scope
          "${pkgs.runCommand "acpi-overrides" { buildInputs = with pkgs; [ cpio ]; } ''
            mkdir -p kernel/firmware/acpi
            cp ${./acpi/ssdt4.aml} kernel/firmware/acpi/ssdt4.aml
            find kernel | cpio -H newc -o > $out
          ''}"
        ];

        extraModprobeConfig = ''
          options iwlwifi amsdu_size=3
          options iwlwifi 11n_disable=8
          options iwlmvm power_scheme=1

          options cfg80211 ieee80211_regdom=US
          options iwlwifi lar_disable=1
        '';
      };
    }

    {
      ${namespace}.services = {
        asusd.enable = true;
        supergfxd.enable = true;
      };
    }

    {
      services.udev.extraRules = ''
        # Samsung 9100 PRO
        ACTION=="add", SUBSYSTEM=="pci", ATTR{class}=="0x010802", ATTR{power/control}="auto"

        # Realtek RTL8111 GbE
        ACTION=="add", SUBSYSTEM=="pci", ATTR{class}=="0x020000", ATTR{power/control}="auto"
      '';
    }

    {
      hardware.graphics = {
        package = overrideMesa pkgs.mesa;
        package32 = overrideMesa pkgs.pkgsi686Linux.mesa;
      };

      environment.sessionVariables = {
        # Prevent some apps from waking the NVIDIA dGPU unnecessarily
        VK_DRIVER_FILES = "/run/opengl-driver/share/vulkan/icd.d/radeon_icd.x86_64.json";
        __EGL_VENDOR_LIBRARY_FILENAMES = "/run/opengl-driver/share/glvnd/egl_vendor.d/50_mesa.json";
      };

      ${namespace}.system.hardware.gpu.nvidia.prime.offload.enableOffloadCmd = false; # Handled manually
      environment.systemPackages = [
        (pkgs.writeShellScriptBin config.hardware.nvidia.prime.offload.offloadCmdMainProgram ''
          export VK_DRIVER_FILES=/run/opengl-driver/share/vulkan/icd.d/nvidia_icd.x86_64.json
          export __EGL_VENDOR_LIBRARY_FILENAMES=/run/opengl-driver/share/glvnd/egl_vendor.d/10_nvidia.json
          export __NV_PRIME_RENDER_OFFLOAD=1
          export __NV_PRIME_RENDER_OFFLOAD_PROVIDER=NVIDIA-G0
          export __GLX_VENDOR_LIBRARY_NAME=nvidia
          export __VK_LAYER_NV_optimus=NVIDIA_only
          exec "$@"
        '')
      ];
    }
  ]);
}

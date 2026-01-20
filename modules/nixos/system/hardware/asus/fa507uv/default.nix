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
    ;
  cfg = config.${namespace}.system.hardware.asus.fa507uv;

  patchesSrc = pkgs.fetchFromGitHub {
    owner = "CachyOS";
    repo = "kernel-patches";
    rev = "bfcf34bd22aa1fa740c5d60a8f126919cfdacfdf";
    hash = "sha256-YdhrS8JBGnM4BvdkG0MbO8I4dJLmF+RyP7VCRCf7LVQ=";
  };

  majorMinor = lib.versions.majorMinor config.${namespace}.system.boot.kernel.packages.kernel.version;

  overrideMesa =
    mesa:
    mesa.overrideAttrs (oldAttrs: {
      mesonFlags = oldAttrs.mesonFlags ++ [
        (pkgs.lib.mesonOption "c_args" "-march=znver4")
        (pkgs.lib.mesonOption "cpp_args" "-march=znver4")
        (pkgs.lib.mesonOption "optimization" "3")
      ];
    });
in
{
  options.${namespace}.system.hardware.asus.fa507uv = {
    enable = mkEnableOption "Whether or not to manage ASUS FA507UV stuff.";
  };

  config = mkIf cfg.enable {
    boot.kernelPatches = [
      {
        name = "asus-armoury-crate";
        patch = "${patchesSrc}/${majorMinor}/0001-asus.patch";
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
        };
      }
    ];

    boot.initrd.prepend = [
      # Fix D3cold power state loop (D0 -> D3cold -> D0), thanks ASUS
      # For reference: Remove Notify (\_SB.NPCF, 0xC0) in the `_OFF` method of \_SB.PCI0.GPP0 scope
      "${pkgs.runCommand "acpi-overrides" { buildInputs = with pkgs; [ cpio ]; } ''
        mkdir -p kernel/firmware/acpi
        cp ${./acpi/ssdt4.aml} kernel/firmware/acpi/ssdt4.aml
        find kernel | cpio -H newc -o > $out
      ''}"
    ];

    boot.extraModprobeConfig = ''
      options cfg80211 ieee80211_regdom=US
      options iwlwifi lar_disable=1
    '';

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
  };
}

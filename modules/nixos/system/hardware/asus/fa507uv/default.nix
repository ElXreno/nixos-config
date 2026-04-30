{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    concatStringsSep
    mkIf
    mkEnableOption
    mkMerge
    ;
  cfg = config.${namespace}.system.hardware.asus.fa507uv;

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
      boot = {
        initrd.prepend = [
          # Fix D3cold power state loop (D0 -> D3cold -> D0), thanks ASUS
          # For reference: Remove Notify (\_SB.NPCF, 0xC0) in the `_OFF` method of \_SB.PCI0.GPP0 scope
          "${pkgs.runCommand "acpi-overrides" { buildInputs = with pkgs; [ cpio ]; } ''
            mkdir -p kernel/firmware/acpi
            cp ${./acpi/ssdt4.aml} kernel/firmware/acpi/ssdt4.aml
            find kernel | cpio -H newc -o > $out
          ''}"
        ];
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
        VK_DRIVER_FILES = concatStringsSep ":" [
          "/run/opengl-driver/share/vulkan/icd.d/radeon_icd.x86_64.json"
          "/run/opengl-driver-32/share/vulkan/icd.d/radeon_icd.i686.json"
        ];
        __EGL_VENDOR_LIBRARY_FILENAMES = concatStringsSep ":" [
          "/run/opengl-driver/share/glvnd/egl_vendor.d/50_mesa.json"
          "/run/opengl-driver-32/share/glvnd/egl_vendor.d/50_mesa.json"
        ];
      };

      ${namespace}.system.hardware.gpu.nvidia.prime.offload.enableOffloadCmd = false; # Handled manually
      environment.systemPackages = [
        (pkgs.writeShellScriptBin config.hardware.nvidia.prime.offload.offloadCmdMainProgram ''
          export VK_DRIVER_FILES=${
            concatStringsSep ":" [
              "/run/opengl-driver/share/vulkan/icd.d/nvidia_icd.x86_64.json"
              "/run/opengl-driver-32/share/vulkan/icd.d/nvidia_icd.i686.json"
            ]
          }
          export __EGL_VENDOR_LIBRARY_FILENAMES=${
            concatStringsSep ":" [
              "/run/opengl-driver/share/glvnd/egl_vendor.d/10_nvidia.json"
              "/run/opengl-driver-32/share/glvnd/egl_vendor.d/10_nvidia.json"
            ]
          }
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

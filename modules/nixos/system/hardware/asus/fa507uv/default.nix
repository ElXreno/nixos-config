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
      ${namespace}.services = {
        asusd.enable = true;
        supergfxd.enable = true;
      };
    }

    {
      services.udev.extraRules = ''
        # Samsung 9100 PRO
        ACTION=="add", SUBSYSTEM=="pci", ATTR{class}=="0x010802", ATTR{power/control}="auto"
      '';
    }

    {
      systemd.services.fa507uv-amdgpu-perf-high = {
        description = "Pin AMD 780M iGPU performance level to high";
        after = [
          "systemd-suspend.service"
          "systemd-hibernate.service"
          "systemd-hybrid-sleep.service"
          "systemd-suspend-then-hibernate.service"
        ];
        wantedBy = [
          "multi-user.target"
          "systemd-suspend.service"
          "systemd-hibernate.service"
          "systemd-hybrid-sleep.service"
          "systemd-suspend-then-hibernate.service"
        ];
        serviceConfig.Type = "oneshot";
        script = ''
          for f in /sys/bus/pci/drivers/amdgpu/*/power_dpm_force_performance_level; do
            [ -w "$f" ] && echo high > "$f" || true
          done
        '';
      };
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

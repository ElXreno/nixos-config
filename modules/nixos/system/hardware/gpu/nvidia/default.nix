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
    mkPackageOption
    mkOption
    filter
    splitString
    concatStringsSep
    elem
    hasInfix
    optional
    optionalAttrs
    optionalString
    types
    versionAtLeast
    ;
  cfg = config.${namespace}.system.hardware.gpu.nvidia;
  igpuIsIntel = config.${namespace}.facts.gpu.intel.exists;

  busIDType = types.strMatching "([[:print:]]+:[0-9]{1,3}(@[0-9]{1,10})?:[0-9]{1,2}:[0-9])?";

  usesOpenModule = cfg.package ? open && cfg.package ? firmware;

  dropClangFromRpath =
    s: concatStringsSep ":" (filter (p: !hasInfix "-clang-" p) (splitString ":" s));

  stripClangRpath =
    pkg:
    pkg.overrideAttrs (
      old:
      optionalAttrs (old ? libPath) { libPath = dropClangFromRpath old.libPath; }
      // optionalAttrs (old ? libPath32 && old.libPath32 != "") {
        libPath32 = dropClangFromRpath old.libPath32;
      }
    );

  basePackage = stripClangRpath cfg.package;

  sysfsConfigInitRootUrl = "https://github.com/NVIDIA/open-gpu-kernel-modules/commit/369378dc1552ae46f7f95a598a085c0025e464b3.patch";

  openPatches =
    (
      if versionAtLeast cfg.package.version "620.73" then
        [
          ./patches/nvidia-drm-ilut-plane-crtc-change.patch
          # NVIDIA/open-gpu-kernel-modules#1572
          (pkgs.fetchpatch {
            name = "nvidia-drm-event-thread-drm-locks.patch";
            url = "https://github.com/NVIDIA/open-gpu-kernel-modules/commit/98fbf975e515a45815efa97b1b3fbaef6f153458.patch";
            hash = "sha256-L691H75NVTsxAd83fA9XIvZlhvAevotPi0AW44X8Ksw=";
          })
        ]
      else
        [
          ./patches/nvidia-drm-colorop-clear-state-changes.patch
          ./patches/nvidia-drm-ilut-crtc-degamma.patch
          ./patches/nvidia-drm-event-thread-drm-locks.patch
        ]
    )
    ++ [
      ./patches/nvidia-drm-output-bpc-displayport.patch
      ./patches/nvidia-drm-zero-dumb-buffers.patch
      (pkgs.fetchpatch {
        name = "nvidia-sysfs-config-init-root-nv-pci.patch";
        url = sysfsConfigInitRootUrl;
        includes = [ "kernel-open/nvidia/nv-pci.c" ];
        hunks = [
          2
          3
        ];
        hash = "sha256-SjN3c3gqs6NbavXrMC7JyiLJUFu/kbTVwRfT8qFh9e0=";
      })
      (pkgs.fetchpatch {
        name = "nvidia-sysfs-config-init-root-kbuild.patch";
        url = sysfsConfigInitRootUrl;
        includes = [ "kernel-open/nvidia/nvidia.Kbuild" ];
        hash = "sha256-nenBdgD2ZtlOcj30JuX1Sa4EFj0rDog6epZveFAYbCk=";
      })
    ]
    ++ optional cfg.acpiNotifyRtd3Guard.enable ./patches/nvidia-dgpu-rtd3-acpi-notify-guards.patch;

  nvidiaPackage =
    if usesOpenModule then
      basePackage.overrideAttrs (old: {
        passthru = (old.passthru or { }) // {
          open = cfg.package.open.overrideAttrs (o: {
            patches = (o.patches or [ ]) ++ openPatches;
            postPatch = (o.postPatch or "") + ''
              substituteInPlace kernel-open/nvidia/nv-pci.c \
                --replace-fail '#include <linux/kthread.h>' $'#include <linux/kthread.h>\n#include <linux/fs_struct.h>'
            '';
          });
        };
      })
    else
      basePackage;

  driverFiles = {
    nvidiaIcd = [
      "/run/opengl-driver/share/vulkan/icd.d/nvidia_icd.json"
      "/run/opengl-driver-32/share/vulkan/icd.d/nvidia_icd.json"
    ];
    nvidiaEgl = [
      "/run/opengl-driver/share/glvnd/egl_vendor.d/10_nvidia.json"
      "/run/opengl-driver-32/share/glvnd/egl_vendor.d/10_nvidia.json"
    ];
    igpuIcd =
      let
        icd = if igpuIsIntel then "intel_icd" else "radeon_icd";
      in
      [
        "/run/opengl-driver/share/vulkan/icd.d/${icd}.x86_64.json"
        "/run/opengl-driver-32/share/vulkan/icd.d/${icd}.i686.json"
      ];
    mesaEgl = [
      "/run/opengl-driver/share/glvnd/egl_vendor.d/50_mesa.json"
      "/run/opengl-driver-32/share/glvnd/egl_vendor.d/50_mesa.json"
    ];
  };

  eglPlatformsWithoutWayland2 = map (f: "/run/opengl-driver/share/egl/egl_external_platform.d/${f}") [
    "10_nvidia_wayland.json"
    "15_nvidia_gbm.json"
    "20_nvidia_xcb.json"
    "20_nvidia_xlib.json"
  ];

  inDriverEnv =
    let
      links = config.systemd.tmpfiles.settings.graphics-driver;
      main = links."/run/opengl-driver"."L+".argument;
      lib32 = links."/run/opengl-driver-32"."L+".argument;
    in
    lib.replaceStrings
      [
        "/run/opengl-driver-32"
        "/run/opengl-driver"
      ]
      [
        (if lib.hasPrefix "/" lib32 then lib32 else main)
        main
      ];

  pinToIgpuGenerator = pkgs.writeShellScript "pin-to-igpu" ''
    for connector in /sys/class/drm/card*-eDP-*; do
      [ "$(${pkgs.coreutils}/bin/cat "$connector/status" 2>/dev/null)" = connected ] || continue
      driver=$(${pkgs.coreutils}/bin/basename "$(${pkgs.coreutils}/bin/readlink -f "$connector/device/device/driver")")
      case "$driver" in
        i915 | xe | amdgpu)
          echo "VK_DRIVER_FILES=${concatStringsSep ":" driverFiles.igpuIcd}"
          echo "__EGL_VENDOR_LIBRARY_FILENAMES=${concatStringsSep ":" driverFiles.mesaEgl}"
          exit 0
          ;;
        nvidia)
          echo "VK_DRIVER_FILES=${concatStringsSep ":" driverFiles.nvidiaIcd}"
          exit 0
          ;;
      esac
    done
  '';

  driverFilesCheck = pkgs.runCommand "nvidia-pin-to-igpu-driver-files" { } ''
    for file in ${lib.escapeShellArgs (map inDriverEnv (lib.concatLists (lib.attrValues driverFiles)))}; do
      [ -e "$file" ] || { echo "missing $file" >&2; exit 1; }
    done
    touch $out
  '';

  eglPlatformsCheck = pkgs.runCommand "nvidia-egl-platforms-without-wayland2" { } ''
    for file in ${lib.escapeShellArgs (map inDriverEnv eglPlatformsWithoutWayland2)}; do
      [ -e "$file" ] || { echo "missing $file" >&2; exit 1; }
    done
    touch $out
  '';
in
{
  options.${namespace}.system.hardware.gpu.nvidia = {
    enable = mkEnableOption "managing nvidia stuff" // {
      default = config.${namespace}.facts.gpu.nvidia.exists;
      defaultText = "Auto-detected from facter report.";
    };
    package = mkPackageOption config.boot.kernelPackages.nvidiaPackages "latest" { };

    powerManagement = {
      enable = mkEnableOption "power management" // {
        default = true;
      };
      finegrained = mkEnableOption "fine-grained power management" // {
        default = cfg.prime.enable;
        defaultText = "Whether PRIME offload is enabled.";
      };
    };

    dynamicBoost.enable = mkEnableOption "nvidia-powerd";
    overclock = {
      enable = mkEnableOption "overclocking NVIDIA GPU";
      freqOffset = mkOption {
        type = types.int;
        description = "GPU core V/F curve offset in MHz.";
      };
      memOffset = mkOption {
        type = types.int;
        default = 0;
        description = "GPU memory clock offset in MHz.";
      };
      maxClock = mkOption {
        type = types.nullOr types.ints.positive;
        default = null;
        description = "Upper GPU core clock lock in MHz, or null to leave clocks unlocked.";
      };
    };
    acpiNotifyRtd3Guard.enable = mkEnableOption "stopping ACPI notifies waking a D3cold dGPU (open module)";
    eglWayland2.enable = mkEnableOption "the egl-wayland2 EGL platform" // {
      default = true;
    };

    moduleParams = mkOption {
      type = with types; attrsOf (either int str);
      default = { };
      description = "Parameters passed to the nvidia kernel module.";
    };

    cudaCapability = mkOption {
      type = types.strMatching "[0-9]+\\.[0-9]+";
      example = "12.0";
      description = "CUDA compute capability of the dGPU that tei, freetoken and strata build and JIT their kernels for.";
    };

    prime = {
      enable = mkEnableOption "Prime support" // {
        default = config.${namespace}.facts.gpu.intel.exists || config.${namespace}.facts.gpu.amd.exists;
        defaultText = "Whether the facter report has an Intel or AMD GPU next to the NVIDIA one.";
      };
      offload.enableOffloadCmd = mkEnableOption "offload cmd" // {
        default = !cfg.prime.pinToIgpu;
      };
      pinToIgpu =
        mkEnableOption "keeping Vulkan and EGL on the iGPU unless launched through the offload command"
        // {
          default = cfg.prime.enable;
          defaultText = "Whether PRIME offload is enabled.";
        };
      intelBusId = mkOption {
        type = busIDType;
        default = config.${namespace}.facts.gpu.intel.busId;
        defaultText = "Derived from facter report.";
      };
      amdgpuBusId = mkOption {
        type = busIDType;
        default = config.${namespace}.facts.gpu.amd.busId;
        defaultText = "Derived from facter report.";
      };
      nvidiaBusId = mkOption {
        type = busIDType;
        default = config.${namespace}.facts.gpu.nvidia.busId;
        defaultText = "Derived from facter report.";
      };
    };

  };

  config = mkIf cfg.enable {
    ${namespace}.system.hardware.gpu.nvidia.moduleParams = {
      NVreg_EnableS0ixPowerManagement = 1;
      NVreg_EnableStreamMemOPs = 1;
      NVreg_EnableResizableBar = 1;
      NVreg_EnablePCIERelaxedOrderingMode = 1;
    };

    environment.variables = {
      __GL_SHADER_DISK_CACHE = "1";
      __GL_SHADER_DISK_CACHE_SKIP_CLEANUP = "1";
    };

    services.xserver.videoDrivers = [
      "nvidia"
    ];

    environment.systemPackages = [
      pkgs.gddr6
    ]
    ++ optional cfg.prime.pinToIgpu (
      pkgs.writeShellScriptBin config.hardware.nvidia.prime.offload.offloadCmdMainProgram ''
        export VK_DRIVER_FILES=${concatStringsSep ":" driverFiles.nvidiaIcd}
        export __EGL_VENDOR_LIBRARY_FILENAMES=${concatStringsSep ":" driverFiles.nvidiaEgl}
        export __NV_PRIME_RENDER_OFFLOAD=1
        export __NV_PRIME_RENDER_OFFLOAD_PROVIDER=NVIDIA-G0
        export __GLX_VENDOR_LIBRARY_NAME=nvidia
        export __VK_LAYER_NV_optimus=NVIDIA_only
        exec "$@"
      ''
    );

    environment.etc."systemd/user-environment-generators/60-pin-to-igpu" = mkIf cfg.prime.pinToIgpu {
      source = pinToIgpuGenerator;
    };

    environment.sessionVariables = mkIf (!cfg.eglWayland2.enable) {
      __EGL_EXTERNAL_PLATFORM_CONFIG_FILENAMES = concatStringsSep ":" eglPlatformsWithoutWayland2;
    };

    system.checks =
      optional cfg.prime.pinToIgpu driverFilesCheck
      ++ optional (!cfg.eglWayland2.enable) eglPlatformsCheck;

    boot.extraModprobeConfig = mkIf (cfg.moduleParams != { }) ''
      options nvidia ${
        concatStringsSep " " (lib.mapAttrsToList (k: v: "${k}=${toString v}") cfg.moduleParams)
      }
    '';

    # Allow /dev/mem access to driver-claimed MMIO (GDDR6 VRAM temp readout via BAR0)
    boot.kernelParams = [ "iomem=relaxed" ];

    boot.initrd.kernelModules =
      mkIf
        (
          config.hardware.facter.enable
          && config.hardware.facter.detected.graphics.enable
          && elem "nvidia" config.hardware.facter.detected.boot.graphics.kernelModules
        )
        [
          "nvidia_modeset"
          "nvidia_drm"
        ];

    services.udev.extraRules = mkIf cfg.powerManagement.finegrained ''
      ACTION=="add", SUBSYSTEM=="pci", DRIVER=="nvidia", ATTR{vendor}=="0x10de", ATTR{class}=="0x030[02]00", TEST=="power/control", ATTR{power/control}="auto"
    '';

    hardware = {
      nvidia = {
        package = nvidiaPackage;
        open = usesOpenModule;
        dynamicBoost.enable = cfg.dynamicBoost.enable;
        powerManagement = {
          inherit (cfg.powerManagement) enable;
          inherit (cfg.powerManagement) finegrained;
        };

        prime = mkIf cfg.prime.enable {
          inherit (cfg.prime) intelBusId amdgpuBusId nvidiaBusId;

          offload = {
            enable = true;
            inherit (cfg.prime.offload) enableOffloadCmd;
          };
        };
      };
    };

    systemd.services = {
      nvidia_oc = mkIf cfg.overclock.enable {
        description = "NVIDIA Overclocking Service";
        after = [ "graphical.target" ];
        wantedBy = [ "graphical.target" ];

        serviceConfig = {
          Type = "oneshot";
          RemainAfterExit = true;
          ExecCondition = "${pkgs.gnugrep}/bin/grep -qxF ${nvidiaPackage.version} /sys/module/nvidia/version";
          ExecStart =
            "${pkgs.nvidia_oc}/bin/nvidia_oc set --index 0 --freq-offset ${toString cfg.overclock.freqOffset}"
            + optionalString (
              cfg.overclock.maxClock != null
            ) " --min-clock 0 --max-clock ${toString cfg.overclock.maxClock}"
            + " --mem-offset ${toString cfg.overclock.memOffset}";
          User = "root";
        };
      };

      nvidia-powerd = mkIf cfg.dynamicBoost.enable {
        restartIfChanged = false;
        serviceConfig.Restart = "always";
      };
    };
  };
}

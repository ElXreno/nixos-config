{
  config,
  inputs,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    mkForce
    types
    literalExpression
    optionals
    ;
  cfg = config.${namespace}.system.boot.kernel;

  ccacheCfg = config.${namespace}.system.ccache;

  applyOptimizations = import ./_build.nix {
    inherit pkgs ccacheCfg;
  };

  # Taken from the stand that measures them rather than kept as a copy here,
  # so that what this machine runs and what the tests prove cannot drift.
  mobilityPatches = inputs.zfs-slab-mobility.patches;
in
{
  options.${namespace}.system.boot.kernel = {
    enable = mkEnableOption "managing kernel" // {
      default = true;
    };
    memAllocProfiling = mkEnableOption "tagging every allocation with its call site";
    flavour = mkOption {
      type = types.enum [
        "xanmod"
        "mainline"
      ];
      default = "mainline";
      description = "Kernel tree that {option}`packages` comes from.";
    };
    packages = mkOption {
      type = types.raw;
      default =
        {
          xanmod = pkgs.linuxPackages_xanmod_latest;
          mainline = pkgs.linuxPackages_7_3;
        }
        .${cfg.flavour};
      defaultText = literalExpression "pkgs.linuxPackages_xanmod_latest or pkgs.linuxPackages_7_3, by flavour";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = pkgs.stdenv.hostPlatform.system == "x86_64-linux";
        message = ''
          The kernel built by ${namespace}.system.boot.kernel supports only x86_64-linux.
          Your system is ${pkgs.stdenv.hostPlatform.system}.
        '';
      }
    ];

    boot = {
      kernelPackages = applyOptimizations cfg.packages;

      kernelPatches = [
        # workaround for https://lore.kernel.org/all/E76BE609-83E4-4FB6-88B8-191F7816EFD6@gmail.com/
        {
          name = "cputime-dyntick-exit-tick";
          patch = ./patches/cputime-dyntick-exit-tick.patch;
        }
        {
          name = "baytrail-clear-native-pin-triggers";
          patch = ./patches/baytrail-clear-native-pin-triggers.patch;
        }
        {
          name = "alc274-msi-alc1316-amps";
          patch = ./patches/alc274-msi-alc1316-amps.patch;
        }
        {
          name = "iwlwifi-by-allow-he-eht";
          patch = ./patches/iwlwifi-by-allow-he-eht.patch;
        }
        {
          name = "pci-aspm-upstream-links-low-power";
          patch = ./patches/pci-aspm-upstream-links-low-power.patch;
        }
        {
          name = "usbhid-clear-halt-after-transaction-error";
          patch = ./patches/usbhid-clear-halt-after-transaction-error.patch;
        }
        {
          name = "drm-edid-adaptive-sync-monitor-range";
          patch = ./patches/drm-edid-adaptive-sync-monitor-range.patch;
        }
        {
          name = "i915-panel-vrr-vtotal-overflow";
          patch = ./patches/i915-panel-vrr-vtotal-overflow.patch;
        }
        {
          name = "i915-vrr-vmax-overflow";
          patch = ./patches/i915-vrr-vmax-overflow.patch;
        }
        {
          name = "i915-vrr-joiner-primary-transcoder";
          patch = ./patches/i915-vrr-joiner-primary-transcoder.patch;
        }
        {
          name = "i915-update-crtc-stages";
          patch = ./patches/i915-update-crtc-stages.patch;
        }
        {
          name = "i915-joined-pipes-single-evasion";
          patch = ./patches/i915-joined-pipes-single-evasion.patch;
        }
        {
          name = "i915-vblank-evade-primary-scanline";
          patch = ./patches/i915-vblank-evade-primary-scanline.patch;
        }
        {
          name = "i915-vrr-pipe-joiner";
          patch = ./patches/i915-vrr-pipe-joiner.patch;
        }
        {
          name = "mac80211-active-monitor-channel-changes";
          patch = pkgs.fetchpatch {
            name = "mac80211-fix-channel-changes-for-active-monitors.patch";
            url = "https://patchwork.kernel.org/project/linux-wireless/patch/20260901025525.4017-1-matheus.aalmeida@inf.ufrgs.br/raw/";
            hash = "sha256-pZRUh2LfOJR0olnO3nBmPUlTrNMLQWfiJXQxI/kbJCw=";
          };
        }
        {
          name = "mac80211-active-monitor-chanctx";
          patch = ./patches/mac80211-active-monitor-chanctx.patch;
        }
        {
          name = "ttm-range-caching-for-large-blocks";
          patch = ./patches/ttm-range-caching-for-large-blocks.patch;
        }
        {
          name = "slab-object-mobility";
          patch = mobilityPatches.kernel.slab-object-mobility;
        }
        {
          name = "filemap-exports";
          patch = mobilityPatches.kernel.filemap-exports;
        }
        {
          name = "compaction-large-folio";
          patch = mobilityPatches.kernel.compaction-large-folio;
        }
        {
          name = "amd-pstate-force-cppc-msr";
          patch = ./patches/amd-pstate-force-cppc.patch;
        }
        {
          name = "faddr2line-llvm-sentinel";
          patch = ./patches/faddr2line-llvm-sentinel.patch;
        }
        {
          name = "thinlto-cache";
          patch = ./patches/thinlto-cache.patch;
        }
        {
          name = "x86_64-version";
          patch = null;
          structuredExtraConfig = with lib.kernel; {
            LTO_CLANG_THIN = yes;
            HIST_TRIGGERS = yes;

            MEM_ALLOC_PROFILING = lib.mkOverride 40 yes;
            MEM_ALLOC_PROFILING_ENABLED_BY_DEFAULT = mkForce no;

            RUST = mkForce unset;
            DRM_PANIC_SCREEN_QR_CODE = mkForce unset;
            NOVA_CORE = mkForce unset;
            DRM_NOVA = mkForce unset;
          };
        }
      ];

      # SLUB folds caches of the same size into one and keeps only the
      # first name, so slabinfo cannot say whether a cache holds zswap
      # entries or trace event fields. A codetag per object can, and
      # slabwho reads it to name what pins a pageblock.
      # Without this the default is "never", which not only leaves profiling off
      # but makes the sysctl read-only. Set from boot because a tag is attached
      # at allocation time, and the caches worth naming grow before login.
      # Not ",compressed": it avoids the page extension array, but falls back to
      # disabling profiling outright when it cannot pack the references.
      kernelParams = optionals cfg.memAllocProfiling [
        "sysctl.vm.mem_profiling=1"
      ];
    };
  };
}

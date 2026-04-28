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
    mkOption
    mkForce
    types
    literalExpression
    optionals
    ;
  cfg = config.${namespace}.system.boot.kernel;

  patchesSrc = pkgs.fetchFromGitHub {
    owner = "CachyOS";
    repo = "kernel-patches";
    rev = "b5e029226df5cc30c103651072d49a7af2878202";
    hash = "sha256-b9Hc0sTxjEzDbphzS9yQqxVha/7bsPIs2cQQQvaG45E=";
  };

  majorMinor = lib.versions.majorMinor config.${namespace}.system.boot.kernel.packages.kernel.version;
  llvm = pkgs.llvmPackages_21;

  clangLLVMStdenv = pkgs.stdenvAdapters.overrideCC llvm.stdenv (
    llvm.stdenv.cc.override {
      bintools = pkgs.wrapBintoolsWith { bintools = llvm.bintools-unwrapped; };
    }
  );

  optimizedStdenv =
    with cfg.optimizations;
    stdenv:
    pkgs.withCFlags (
      additionalFlags
      ++ [
        "-march=znver${toString znver}"
        # Disabled due -Werror=format-truncation=
        # "-O${optLevel}"
      ]
    ) stdenv;

  applyOptimizations =
    kernelPackages:
    kernelPackages.extend (
      self: super: {
        kernel =
          (super.kernel.override {
            stdenv = optimizedStdenv clangLLVMStdenv;
          }).overrideAttrs
            (prevAttrs: {
              pname = "${prevAttrs.pname}-znver${toString cfg.optimizations.znver}";
            });

        # TODO: Upstream to nixpkgs
        zenpower = super.zenpower.overrideAttrs (prev: {
          makeFlags = (prev.makeFlags or [ ]) ++ self.kernelModuleMakeFlags;
        });
      }
    );

  finalKernelPackages =
    if cfg.optimizations.enable then (applyOptimizations cfg.packages) else cfg.packages;
in
{
  options.${namespace}.system.boot.kernel = {
    enable = mkEnableOption "Whether or not to manage kernel." // {
      default = true;
    };
    packages = mkOption {
      type = types.raw;
      default =
        if config.${namespace}.roles.laptop.enable then
          pkgs.linuxPackages_xanmod_edge
        else
          pkgs.linuxPackages_latest;
      defaultText = literalExpression "pkgs.linuxPackages_latest";
    };
    optimizations = {
      enable = mkEnableOption "Whether to build optimized kernel.";
      znver = mkOption {
        type = with types; enum (lib.lists.range 1 4);
        default = 2;
        example = 4;
      };
      optLevel = mkOption {
        type = types.enum [
          "0"
          "1"
          "2"
          "3"
          "s"
          "z"
          "fast"
        ];
        default = "3";
        description = ''
          Ref: https://gist.github.com/lolo32/fd8ce29b218ac2d93a9e
        '';
      };
      additionalFlags = mkOption {
        type = with types; listOf str;
        default = [ "-pipe" ];
      };
    };
  };

  config = mkIf cfg.enable {
    assertions = optionals cfg.optimizations.enable [
      {
        assertion = pkgs.stdenv.hostPlatform.system == "x86_64-linux";
        message = ''
          Kernel optimizations which is enabled via ${namespace}.system.boot.kernel.optimizations.enable
          supported only on x86_64-linux systems. Your system is ${pkgs.stdenv.hostPlatform.system}.
        '';
      }
    ];

    boot = {
      kernelPackages = finalKernelPackages;

      kernelPatches = optionals cfg.optimizations.enable [
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
          name = "mac80211-tdls-allow-key-install-before-assoc";
          patch = ./kernel-patches/0001-wifi-mac80211-allow-key-installation-for-TDLS-peers-.patch;
        }
        {
          name = "x86_64-version";
          patch = null;
          structuredExtraConfig = with lib.kernel; {
            "MZEN${toString cfg.optimizations.znver}" = yes;
            LTO_NONE = mkForce unset;
            LTO_CLANG_THIN = yes;

            RUST = mkForce unset;
            DRM_PANIC_SCREEN_QR_CODE = mkForce unset;
            NOVA_CORE = mkForce unset;
            DRM_NOVA = mkForce unset;

            ASUS_ARMOURY = module;
            NTSYNC = module;

            DEBUG_LIST = mkForce no;
          };
        }
      ];
    };
  };
}

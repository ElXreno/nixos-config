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
        if (config.${namespace}.roles.laptop.enable || cfg.optimizations.enable) then
          pkgs.linuxPackages_xanmod_latest
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

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
    types
    literalExpression
    optionals
    ;
  cfg = config.${namespace}.system.boot.kernel;

  optimizedStdenv =
    with cfg.optimizations;
    stdenv:
    pkgs.withCFlags (
      additionalFlags
      ++ [
        "-march=x86-64-v${toString isa}"
        # Disabled due -Werror=format-truncation=
        # "-O${optLevel}"
      ]
    ) stdenv;

  applyOptimizations =
    kernelPackages:
    kernelPackages.extend (
      self: super: {
        stdenv = optimizedStdenv super.stdenv;
        kernel = super.kernel.overrideAttrs (
          _finalAttrs: prevAttrs: {
            pname = "${prevAttrs.pname}-x86-64-v${toString cfg.optimizations.isa}";
          }
        );
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
      default = pkgs.linuxPackages_latest;
      defaultText = literalExpression "pkgs.linuxPackages_latest";
    };
    optimizations = {
      enable = mkEnableOption "Whether to build optimized kernel.";
      isa = mkOption {
        type = with types; nullOr (enum (lib.lists.range 1 3));
        default = null;
        example = 4;
        description = ''
          For x86-64 watch this: https://en.opensuse.org/X86-64_microarchitecture_levels
          Fast check: `ld.so --help | grep x86-64` or `, inxi -aCz | grep level`

          x86-64-v4 is not included since the kernel does not use AVX512 instructions.
        '';
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
        assertion = pkgs.system == "x86_64-linux";
        message = ''
          Kernel optimizations which is enabled via ${namespace}.system.boot.kernel.optimizations.enable
          supported only on x86_64-linux systems. Your system is ${pkgs.system}.
        '';
      }
      {
        assertion = cfg.optimizations.isa != null;
        message = ''
          When ${namespace}.system.boot.kernel.optimizations.enable is true,
          ${namespace}.system.boot.kernel.optimizations.isa must be set.
        '';
      }
    ];

    boot = {
      kernelPackages = finalKernelPackages;

      kernelPatches = optionals cfg.optimizations.enable [
        {
          name = "x86_64-version";
          patch = null;
          structuredExtraConfig = {
            X86_64_VERSION = lib.kernel.freeform "${toString cfg.optimizations.isa}";
          };
        }
      ];
    };
  };
}

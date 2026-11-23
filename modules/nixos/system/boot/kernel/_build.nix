{
  pkgs,
  buildPkgs,
  ccacheCfg,
}:
let
  llvm = pkgs.llvmPackages_22;

  clangLLVMStdenv = pkgs.stdenvAdapters.overrideCC llvm.stdenv (
    llvm.stdenv.cc.override {
      bintools = pkgs.wrapBintoolsWith { bintools = llvm.bintools-unwrapped; };
    }
  );

  optimizedStdenv = pkgs.withCFlags [ "-pipe" ];

  withCcache = import ./_ccache.nix {
    inherit ccacheCfg;
    inherit (pkgs) ccacheTools;
    pkgs = buildPkgs;
  };
in
kernelPackages:
kernelPackages.extend (
  self: super: {
    kernel = withCcache (
      if super.kernel.stdenv.cc.isClang then super.kernel.stdenv else optimizedStdenv clangLLVMStdenv
    ) super.kernel;

    # TODO: Upstream to nixpkgs
    zenpower = super.zenpower.overrideAttrs (prev: {
      patches = (prev.patches or [ ]) ++ [
        # https://github.com/mattkeenan/zenpower5/pull/8
        (pkgs.fetchpatch {
          url = "https://github.com/mattkeenan/zenpower5/commit/0ba5635620082047f61bc9663de2d4dd5fc98c56.patch";
          hash = "sha256-m5G/4vg2/9uPpR2DclvvGq3rZ2u01qYymNVrbMshrgw=";
        })
      ];
      makeFlags = (prev.makeFlags or [ ]) ++ self.kernelModuleMakeFlags;
    });

    ryzen-smu = super.ryzen-smu.overrideAttrs (prev: {
      makeFlags = (prev.makeFlags or [ ]) ++ self.kernelModuleMakeFlags;
    });
  }
)

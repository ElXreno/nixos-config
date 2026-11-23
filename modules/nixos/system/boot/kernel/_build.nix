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
  _self: super: {
    kernel = withCcache (
      if super.kernel.stdenv.cc.isClang then super.kernel.stdenv else optimizedStdenv clangLLVMStdenv
    ) super.kernel;
  }
)

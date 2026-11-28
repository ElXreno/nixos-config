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

  pahole = pkgs.buildPackages.pahole-merged-cu;

  buildLinux = pkgs.buildLinux.override {
    inherit pahole;
    callPackage = pkgs.newScope { inherit pahole; };
  };

  withPahole =
    kernel:
    if kernel.override.__functionArgs ? buildLinux then
      kernel.override { inherit buildLinux; }
    else
      kernel;
in
kernelPackages:
kernelPackages.extend (
  _self: super: {
    kernel = withCcache (
      if super.kernel.stdenv.cc.isClang then super.kernel.stdenv else optimizedStdenv clangLLVMStdenv
    ) (withPahole super.kernel);
  }
)

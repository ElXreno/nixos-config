{
  pkgs,
  ccacheCfg,
}:
let
  llvm = pkgs.llvmPackages_22;

  clangLLVMStdenv = pkgs.stdenvAdapters.overrideCC llvm.stdenv (
    llvm.stdenv.cc.override {
      bintools = pkgs.wrapBintoolsWith { bintools = llvm.bintools-unwrapped; };
    }
  );

  cachedStdenv = ccacheCfg.wrapStdenv clangLLVMStdenv;

  optimizedStdenv = pkgs.withCFlags [ "-pipe" ];

  # workaround for https://github.com/NixOS/nixpkgs/commit/42f4cc3a70dfee64ba246bcb2be5764414432201
  kernelCC = pkgs.writeShellScript "kernel-cc" ''
    if [ -n "''${KSRC_REAL-}" ]; then
      args=()
      for arg; do
        args+=("''${arg//"$KSRC_REAL"/"$KSRC_LINK"}")
      done
      set -- "''${args[@]}"
    fi
    exec ${pkgs.lib.getExe cachedStdenv.cc.cc} "$@"
  '';
in
kernelPackages:
kernelPackages.extend (
  self: super: {
    kernel =
      (super.kernel.override (prevArgs: {
        stdenv = optimizedStdenv cachedStdenv;
        extraMakeFlags = (prevArgs.extraMakeFlags or [ ]) ++ [
          "CC=${kernelCC}"
          "KBUILD_THINLTO_CACHE=${ccacheCfg.thinltoCacheDir}"
        ];
      })).overrideAttrs
        (prev: {
          preConfigure = (prev.preConfigure or "") + ''
            export KSRC_REAL=$PWD KSRC_LINK=$NIX_BUILD_TOP/ksrc
            ln -s -- "$KSRC_REAL" "$KSRC_LINK"
            echo "kernel-cc: $KSRC_REAL -> $KSRC_LINK"
          '';
        });

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

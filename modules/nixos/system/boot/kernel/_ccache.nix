{
  pkgs,
  ccacheTools,
  ccacheCfg,
}:
let
  tools = ccacheTools.override {
    inherit pkgs;
    inherit (ccacheCfg) cacheDir settings;
  };
in
stdenv: kernel:
let
  cachedStdenv = tools.wrapStdenv stdenv;

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
(kernel.override (prevArgs: {
  stdenv = cachedStdenv;
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
  })

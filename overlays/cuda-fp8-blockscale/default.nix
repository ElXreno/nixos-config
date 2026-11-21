_:

final: prev:
let
  inherit (prev) lib;

  wrapper = final.callPackage ../../packages/ptxas-fp8-blockscale { };

  wrapToolkit =
    cprev:
    cprev.cudatoolkit.overrideAttrs (old: {
      nativeBuildInputs = (old.nativeBuildInputs or [ ]) ++ [ final.makeBinaryWrapper ];
      buildCommand = old.buildCommand + ''
        rm "$out/bin/ptxas"
        makeBinaryWrapper ${lib.getExe wrapper} "$out/bin/ptxas" \
          --set PTXAS_FP8_REAL ${lib.getBin cprev.cuda_nvcc}/bin/ptxas
      '';
      passthru = old.passthru // {
        ptxasFp8BlockScale = true;
      };
    });

  wrapPtxas = _: cprev: {
    cudatoolkit =
      if
        lib.versionAtLeast cprev.cudaMajorMinorVersion "12.8"
        && !(cprev.cudatoolkit.ptxasFp8BlockScale or false)
      then
        wrapToolkit cprev
      else
        cprev.cudatoolkit;
  };
in
{
  _cuda = prev._cuda.extend (
    _: cudaPrev: {
      extensions = cudaPrev.extensions ++ [ wrapPtxas ];
    }
  );
}

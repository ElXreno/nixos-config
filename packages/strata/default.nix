{
  lib,
  fetchFromGitHub,
  autoAddDriverRunpath,
  cudaPackages,
  cmake,
  ninja,
  ccacheTools,
  python3,
  makeWrapper,
}:
let
  strata = lib.makeOverridable (
    {
      cudaCapabilities ? [ "8.9" ],
      ggmlCpuFeatures ? [
        "AVX512"
        "AVX512_VBMI"
        "AVX512_VNNI"
        "AVX512_BF16"
      ],
    }:
    let
      python = python3.withPackages (
        ps: with ps; [
          jinja2
          numpy
          pillow
          psutil
          pyyaml
          regex
          requests
          tqdm
        ]
      );

      cudaArchitectures = lib.concatMapStringsSep ";" (lib.replaceStrings
        [ "." ]
        [ "" ]
      ) cudaCapabilities;
    in
    (ccacheTools.wrapStdenv cudaPackages.backendStdenv).mkDerivation (finalAttrs: {
      pname = "strata";
      version = "0.2.20-unstable-2026-11-24";

      src = fetchFromGitHub {
        owner = "ElXreno";
        repo = "Strata";
        rev = "d4f219ac8e436d847405005fe739e4879ebb2ecf";
        hash = "sha256-bQwWD/ywM4MAjrXKHyE9tyAn1Bqn1yJC8NGOYxP7yTw=";
      };

      nativeBuildInputs = [
        autoAddDriverRunpath
        cmake
        ninja
        makeWrapper
        cudaPackages.cuda_nvcc
      ];

      buildInputs = with cudaPackages; [
        cccl
        cuda_cudart
        nvtx
        cuda_profiler_api
        libcublas
      ];

      cmakeFlags = [
        (lib.cmakeBool "STRATA_ENABLE_CUDA" true)
        (lib.cmakeBool "STRATA_NATIVE_EXPERTS" true)
        (lib.cmakeBool "STRATA_BUILD_TESTS" false)
        (lib.cmakeBool "STRATA_PORTABLE" true)
      ]
      ++ map (feature: lib.cmakeBool "GGML_${feature}" true) ggmlCpuFeatures
      ++ [
        (lib.cmakeFeature "CMAKE_CUDA_ARCHITECTURES" cudaArchitectures)
        (lib.cmakeFeature "CMAKE_CUDA_COMPILER_LAUNCHER" "${ccacheTools.launcher}")
      ];

      cmakeDir = "../engine";

      ninjaFlags = [ "strata" ];

      installPhase = ''
        runHook preInstall

        install -Dm755 strata $out/bin/strata

        share=$out/share/strata
        mkdir -p $share
        cp -r $src/. $share/
        chmod -R u+w $share

        makeWrapper ${python.interpreter} $out/bin/strata-serve \
          --add-flags $share/terminal/serve/server.py
        makeWrapper ${python.interpreter} $out/bin/strata-pack \
          --add-flags $share/terminal/tools/iq_pack.py

        runHook postInstall
      '';

      passthru = {
        inherit cudaCapabilities ggmlCpuFeatures;
      };

      meta = {
        description = "Inference engine running Qwen3.8-Flash-Next from VRAM, RAM and NVMe expert tiers";
        homepage = "https://github.com/ElXreno/Strata";
        license = lib.licenses.mit;
        platforms = [ "x86_64-linux" ];
        mainProgram = "strata-serve";
      };
    })
  ) { };
in
{
  inherit strata;
  strata-sm120 = strata.override {
    cudaCapabilities = [ "12.0" ];
    ggmlCpuFeatures = [ "AVX_VNNI" ];
  };
}

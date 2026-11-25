{
  lib,
  fetchFromGitHub,
  applyPatches,
  autoAddDriverRunpath,
  cudaPackages_13_0,
  cmake,
  ninja,
  git,
  ccacheTools,
  python3,
  makeWrapper,
  cudaCapabilities ? [ "12.0" ],
  ggmlCpuFeatures ? [ "AVX_VNNI" ],
}:
let
  cudaPackages = cudaPackages_13_0;

  src = fetchFromGitHub {
    owner = "ElXreno";
    repo = "Xolotl";
    rev = "fb4fe266475a04f750a9c425f19e6a6cf5307a51";
    hash = "sha256-+FS2exOtAdlb1LmymqgdS4pfhr0Fyu6c9DDt8Tn7tlE=";
  };

  llamaCpp = applyPatches {
    name = "llama.cpp-xolotl";
    src = fetchFromGitHub {
      owner = "ggml-org";
      repo = "llama.cpp";
      rev = "6050b467c07a95c7b87debd4ddac2f8ba8d9b807";
      hash = "sha256-MLaDdiMcu3GwitwwuatrUf0onFaXxzOKajfDxWSk5Ec=";
    };
    patches = [ "${src}/tools/xolotl/ggml/xolotl-ggml.patch" ];
  };

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
(ccacheTools.wrapStdenv cudaPackages.backendStdenv).mkDerivation {
  pname = "xolotl";
  version = "0-unstable-2026-11-26";

  inherit src;

  nativeBuildInputs = [
    autoAddDriverRunpath
    cmake
    ninja
    git
    makeWrapper
    cudaPackages.cuda_nvcc
  ];

  buildInputs = with cudaPackages; [
    cccl
    cuda_cudart
    cuda_culibos
    nvtx
    cuda_profiler_api
    libcublas
    libcublas.static
  ];

  cmakeFlags = [
    (lib.cmakeBool "STRATA_ENABLE_CUDA" true)
    (lib.cmakeBool "STRATA_GGML_CUDA" true)
    (lib.cmakeBool "STRATA_MMQ_KQUANTS" true)
    (lib.cmakeBool "STRATA_DSV41_FQ_OP" true)
    (lib.cmakeBool "STRATA_DSV41_HC_MIX" true)
    (lib.cmakeBool "STRATA_BUILD_TESTS" false)
    (lib.cmakeBool "STRATA_PORTABLE" true)
  ]
  ++ map (feature: lib.cmakeBool "GGML_${feature}" true) ggmlCpuFeatures
  ++ [
    (lib.cmakeFeature "STRATA_GGML_DIR" "${llamaCpp}")
    (lib.cmakeFeature "CMAKE_CUDA_ARCHITECTURES" cudaArchitectures)
    (lib.cmakeFeature "CMAKE_CUDA_COMPILER_LAUNCHER" "${ccacheTools.launcher}")
  ];

  ninjaFlags = [ "ds41_generate" ];

  installPhase = ''
    runHook preInstall

    install -Dm755 ds41_generate $out/bin/ds41_generate

    share=$out/share/xolotl
    mkdir -p $share
    cp -r $src/. $share/
    chmod -R u+w $share
    rm -rf $share/third_party/llama.cpp
    ln -s ${llamaCpp} $share/third_party/llama.cpp
    mkdir -p $share/build-xolotl
    ln -s $out/bin/ds41_generate $share/build-xolotl/ds41_generate

    makeWrapper ${python.interpreter} $out/bin/xolotl-ds41-serve \
      --add-flags $share/tools/v41/serve/serve_ds41.py
    makeWrapper ${python.interpreter} $out/bin/xolotl-extract-engram \
      --add-flags $share/tools/v41/extract_engram.py

    runHook postInstall
  '';

  passthru = {
    inherit cudaCapabilities ggmlCpuFeatures llamaCpp;
  };

  meta = {
    description = "DeepSeek-V4.1-Flash engine with experts tiered across VRAM, pinned RAM and NVMe";
    homepage = "https://github.com/Indras-Mirror/Xolotl";
    license = lib.licenses.mit;
    platforms = [ "x86_64-linux" ];
    mainProgram = "ds41_generate";
  };
}

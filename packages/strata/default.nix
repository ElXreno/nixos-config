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
  cudaCapabilities ? [ "8.9" ],
  ggmlCpuFeatures ? [
    "AVX512"
    "AVX512_VBMI"
    "AVX512_VNNI"
    "AVX512_BF16"
  ],
  withDsv41 ? false,
}:
let
  llamaCpp = fetchFromGitHub {
    owner = "ggml-org";
    repo = "llama.cpp";
    rev = "3cf03257f219afbe7334045ff7c6a06ac68c627d";
    hash = "sha256-SRGoXa+4ACBCB3eaG9XFYhMN1i0FyPEy9Rrer+dFGYI=";
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
(ccacheTools.wrapStdenv cudaPackages.backendStdenv).mkDerivation (finalAttrs: {
  pname = "strata";
  version = "0.2.13.1-unstable-2026-11-14";

  src = fetchFromGitHub {
    owner = "ElXreno";
    repo = "Strata";
    rev = "30bebf8356e6e00c58a3bd7b9d8ee4481e693158";
    hash = "sha256-U8olQV/o5eazySwAUnm2L7DiStZU5cAtX3rscbW+oik=";
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
    cuda_nvtx
    cuda_profiler_api
    libcublas
  ];

  cmakeFlags = [
    (lib.cmakeBool "STRATA_ENABLE_CUDA" true)
    (lib.cmakeBool "STRATA_NATIVE_EXPERTS" true)
    (lib.cmakeBool "STRATA_BUILD_TESTS" false)
    (lib.cmakeBool "STRATA_PORTABLE" true)
    (lib.cmakeBool "STRATA_BUILD_DSV41" withDsv41)
  ]
  ++ map (feature: lib.cmakeBool "GGML_${feature}" true) ggmlCpuFeatures
  ++ [
    (lib.cmakeFeature "STRATA_GGML_DIR" "${llamaCpp}")
    (lib.cmakeFeature "CMAKE_CUDA_ARCHITECTURES" cudaArchitectures)
    (lib.cmakeFeature "CMAKE_CUDA_COMPILER_LAUNCHER" "${ccacheTools.launcher}")
  ];

  ninjaFlags = [ "strata" ] ++ lib.optional withDsv41 "strata-dsv41";

  installPhase = ''
    runHook preInstall

    install -Dm755 strata $out/bin/strata

    share=$out/share/strata
    mkdir -p $share
    cp -r $src/. $share/
    chmod -R u+w $share
    ln -s ${llamaCpp} $share/third_party/llama.cpp

    makeWrapper ${python.interpreter} $out/bin/strata-serve \
      --add-flags $share/serve/server.py
    makeWrapper ${python.interpreter} $out/bin/strata-pack \
      --add-flags $share/tools/iq_pack.py
  ''
  + lib.optionalString withDsv41 ''
    install -Dm755 strata-dsv41 $out/bin/strata-dsv41
    makeWrapper ${python.interpreter} $out/bin/strata-dsv41-pack \
      --add-flags $share/tools/dsv41/pack_gguf.py
  ''
  + ''

    runHook postInstall
  '';

  passthru = {
    inherit cudaCapabilities ggmlCpuFeatures withDsv41;
  };

  meta = {
    description = "Inference engine running Qwen3.8-Flash-Next and DeepSeek-V4.1-Flash from VRAM, RAM and NVMe expert tiers";
    homepage = "https://github.com/ElXreno/Strata";
    license = lib.licenses.mit;
    platforms = [ "x86_64-linux" ];
    mainProgram = "strata-serve";
  };
})

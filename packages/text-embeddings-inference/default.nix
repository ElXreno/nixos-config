{
  lib,
  rustPlatform,
  fetchFromGitHub,
  pkg-config,
  autoPatchelfHook,
  autoAddDriverRunpath,
  aws-lc,
  oniguruma,
  openssl,
  ccacheTools,
  cudaPackages_12_9,
  nix-update-script,
}:
let
  text-embeddings-inference = lib.makeOverridable (
    {
      cudaCapability ? "8.9",
    }:
    let
      cudaPackages = cudaPackages_12_9;
    in
    rustPlatform.buildRustPackage rec {
      pname = "text-embeddings-inference";
      version = "1.9.4";

      __structuredAttrs = true;
      strictDeps = true;

      src = fetchFromGitHub {
        owner = "huggingface";
        repo = pname;
        rev = "v${version}";
        hash = "sha256-4sxL47jrKqcvFHafwix2l+a+44neDLw+DrZw89ywjtc=";
      };

      cargoHash = "sha256-mMAzKHzoXm6JEQGJRk4aPIVhr/WdbNB6qTkrcRWctyQ=";

      buildAndTestSubdir = "router";
      buildNoDefaultFeatures = true;
      buildFeatures = [
        "http"
        "candle"
        "candle-cuda"
        "dynamic-linking"
      ];

      nativeBuildInputs = [
        pkg-config
        autoPatchelfHook
        autoAddDriverRunpath
        (ccacheTools.wrapCompiler cudaPackages.cuda_nvcc "nvcc")
        cudaPackages.cuda_nvcc
      ];

      buildInputs = [
        aws-lc
        oniguruma
        openssl
        cudaPackages.cccl
        cudaPackages.cuda_cudart
        cudaPackages.cuda_nvrtc
        cudaPackages.libcublas
        cudaPackages.libcurand
      ];

      env = {
        CFLAGS = "-std=gnu17";
        CUDA_COMPUTE_CAP = lib.toInt (lib.replaceStrings [ "." ] [ "" ] cudaCapability);
        CUDA_TOOLKIT_ROOT_DIR = lib.getDev cudaPackages.cuda_cudart;
      };

      preBuild = ''
        export RAYON_NUM_THREADS=$NIX_BUILD_CORES
      '';

      appendRunpaths = [ (lib.makeLibraryPath [ cudaPackages.libcublas ]) ];
      autoPatchelfIgnoreMissingDeps = [ "libcuda.so.1" ];

      doCheck = false;

      passthru.updateScript = nix-update-script { };

      meta = {
        description = "High-performance text embeddings inference (CUDA)";
        homepage = "https://github.com/huggingface/text-embeddings-inference";
        license = lib.licenses.asl20;
        platforms = lib.platforms.linux;
        mainProgram = "text-embeddings-router";
      };
    }
  ) { };
in
{
  inherit text-embeddings-inference;
  text-embeddings-inference-sm120 = text-embeddings-inference.override { cudaCapability = "12.0"; };
}

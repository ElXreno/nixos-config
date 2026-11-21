{
  lib,
  fetchFromGitHub,
  fetchurl,
  fetchPypi,
  python313,
  cudaPackages_13,
  autoAddDriverRunpath,
  addDriverRunpath,
  autoPatchelfHook,
  ccacheTools,
  ninja,
  makeWrapper,
  runCommand,
  stdenv,
  zlib,
  cudaCapabilities ? [ "8.9" ],
}:
let
  cudaPackages = cudaPackages_13;

  cudaHome = runCommand "freetoken-cuda-home-${cudaPackages.cudaMajorMinorVersion}" { } ''
    mkdir -p $out
    ln -s ${cudaPackages.cudatoolkit}/* $out/
    ln -s ${cudaPackages.cudatoolkit}/lib $out/lib64
  '';

  python = python313.override {
    self = python;
    packageOverrides = final: prev: {
      cuda-bindings = prev.cuda-bindings.override { inherit cudaPackages; };

      compressed-tensors = prev.compressed-tensors.overridePythonAttrs (old: {
        dependencies = (old.dependencies or [ ]) ++ [ final.psutil ];
        doCheck = false;
      });

      einops = prev.einops.overridePythonAttrs (_old: {
        doCheck = false;
      });

      llguidance = prev.llguidance.overridePythonAttrs (_old: {
        doCheck = false;
      });

      triton = final.buildPythonPackage {
        pname = "triton";
        version = "3.8.0";
        format = "wheel";

        src = fetchurl {
          url = "https://files.pythonhosted.org/packages/b0/b6/3394d5548404c1cabd1dadadd28d0b3f9478db1dff8180da53bb3f0a1e19/triton-3.8.0-cp313-cp313-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl";
          hash = "sha256-HwSXIY4mt9eXc62cKj+jtTnuafWHoT+sLlUrHTIqgBU=";
        };

        pythonRemoveDeps = [
          "cmake"
          "torch"
        ];

        buildInputs = [ zlib ];

        nativeBuildInputs = [ autoPatchelfHook ];

        dontStrip = true;

        postFixup = ''
          mkdir -p $out/${final.python.sitePackages}/triton/third_party/cuda/bin/
          ln -s ${lib.getBin cudaPackages.cuda_nvcc}/bin/ptxas \
            $out/${final.python.sitePackages}/triton/third_party/cuda/bin/
          substituteInPlace "$out/${final.python.sitePackages}/triton/backends/nvidia/driver.py" \
            --replace-fail \
              "if env_libcuda_path := knobs.nvidia.libcuda_path:" \
              "return ['${addDriverRunpath.driverLink}/lib']" \
            --replace-fail \
              "return [env_libcuda_path]" \
              ""
        '';

        meta = {
          description = "Language and compiler for custom Deep Learning operations";
          homepage = "https://github.com/triton-lang/triton/";
          license = lib.licenses.mit;
          platforms = [ "x86_64-linux" ];
        };
      };

      torch =
        (prev.torch-bin.override {
          inherit cudaPackages;
          inherit (final) triton;
        }).overrideAttrs
          (old: {
            version = "2.14.1+cu130";
            src = fetchurl {
              name = "torch-2.14.1+cu130-cp313-cp313-manylinux_2_28_x86_64.whl";
              url = "https://download.pytorch.org/whl/cu130/torch-2.14.1%2Bcu130-cp313-cp313-manylinux_2_28_x86_64.whl";
              hash = "sha256-OnjqCk1mBcHjUbM6uKq+RN24FSOc7ADMb4VVXSbApaU=";
            };
            pythonRelaxDeps = [ "setuptools" ];
            buildInputs = (old.buildInputs or [ ]) ++ [ cudaPackages.libcurand ];
            appendRunpaths = (old.appendRunpaths or [ ]) ++ [ "${addDriverRunpath.driverLink}/lib" ];
            passthru = old.passthru // {
              inherit cudaCapabilities;
              cudaSupport = true;
              rocmSupport = false;
            };
          });

      torchvision = (prev.torchvision-bin.override { torch-bin = final.torch; }).overrideAttrs (_old: {
        version = "0.29.1+cu130";
        src = fetchurl {
          name = "torchvision-0.29.1+cu130-cp313-cp313-manylinux_2_28_x86_64.whl";
          url = "https://download.pytorch.org/whl/cu130/torchvision-0.29.1%2Bcu130-cp313-cp313-manylinux_2_28_x86_64.whl";
          hash = "sha256-R/AArLJEmygdQ3B8kBFqMMf6ACx/e79pdrzCVi3VeHs=";
        };
      });

      flashinfer-python =
        (prev.flashinfer-python.override { inherit cudaPackages; }).overridePythonAttrs
          (old: {
            postPatch = (old.postPatch or "") + ''
              substituteInPlace csrc/batch_prefill_paged_kernel_inst.jinja csrc/batch_prefill_ragged_kernel_inst.jinja \
                --replace-fail "[16, 64, 128]" "[16, 32, 64, 128]"
            '';
          });

      huggingface-hub = prev.huggingface-hub.overridePythonAttrs (_old: rec {
        version = "1.33.0";
        src = fetchPypi {
          pname = "huggingface_hub";
          inherit version;
          hash = "sha256-NnviGiAduVI+3fiurHBI8mAsGzCGkcl2QNXnLtGIAH4=";
        };
        doCheck = false;
      });

      transformers = prev.transformers.overridePythonAttrs (old: rec {
        version = "5.18.0";
        src = fetchPypi {
          inherit (old) pname;
          inherit version;
          hash = "sha256-2JwgbkLoQa980xSz0rIcA+l32BnNtK72WtoEh2mmRN8=";
        };
        doCheck = false;
      });

      flashlib = final.buildPythonPackage rec {
        pname = "flashlib";
        version = "0.4.0";
        pyproject = true;

        src = fetchPypi {
          inherit pname version;
          hash = "sha256-hWAI4S3Y6YVUcljhBpYp5or1MPr4u5u9M+J5N6P6TBA=";
        };

        build-system = [ final.setuptools ];

        dependencies = with final; [
          numpy
          torch
          tqdm
          triton
        ];

        pythonRelaxDeps = [ "numpy" ];
        pythonRemoveDeps = [ "nvidia-cutlass-dsl" ];

        pythonImportsCheck = [ "flashlib.kernels.slot_cache" ];

        doCheck = false;

        meta = {
          description = "High-performance ML primitives: Triton and CuteDSL kernels for NVIDIA GPUs";
          homepage = "https://github.com/FlashML-org/flashlib";
          license = lib.licenses.asl20;
          platforms = [ "x86_64-linux" ];
        };
      };
    };
  };
in
(python.pkgs.buildPythonApplication.override { stdenv = ccacheTools.wrapStdenv stdenv; })
  (finalAttrs: {
    pname = "freetoken";
    version = "0.2.2-unstable-2026-11-11";
    pyproject = true;

    src = fetchFromGitHub {
      owner = "ElXreno";
      repo = "FreeToken";
      rev = "243e6331ce3e5db8d9dabd288b45a53fce02f339";
      hash = "sha256-3Yb+mrW3MdIAnfVNdHtBYGrXCafqNR4b6dQ5GKvPH5k=";
    };

    build-system = with python.pkgs; [
      setuptools
      torch
      wheel
    ];

    nativeBuildInputs = [
      autoAddDriverRunpath
      makeWrapper
      ninja
      (lib.getBin cudaPackages.cuda_nvcc)
    ];

    buildInputs = [ cudaPackages.cuda_cudart ];

    dependencies = with python.pkgs; [
      apache-tvm-ffi
      einops
      fastapi
      flashinfer-python
      flashlib
      gguf
      huggingface-hub
      msgpack
      numpy
      openai
      partial-json-parser
      pillow
      prompt-toolkit
      pydantic
      pyzmq
      safetensors
      torch
      torchvision
      tqdm
      transformers
      triton
      uvicorn
      xgrammar
    ];

    env = {
      CUDA_HOME = "${cudaHome}";
      PYTORCH_NVCC = "${ccacheTools.wrapCompiler cudaPackages.cuda_nvcc "nvcc"}/bin/nvcc";
      TORCH_NO_COMPILER_WRAPPER = "1";
    };

    pythonRelaxDeps = [
      "apache-tvm-ffi"
      "gguf"
      "numpy"
      "openai"
      "pillow"
      "transformers"
    ];

    pythonRemoveDeps = [ "modelscope" ];

    postFixup = ''
      wrapProgram $out/bin/ft \
        --prefix PATH : ${
          lib.makeBinPath [
            (lib.getBin cudaPackages.cuda_nvcc)
            ninja
            stdenv.cc
          ]
        } \
        --set-default CUDA_HOME ${cudaHome} \
        --set-default NVCC_PREPEND_FLAGS "-I${cudaHome}/include -L${cudaHome}/lib" \
        --set-default TRITON_PTXAS_PATH ${lib.getBin cudaPackages.cuda_nvcc}/bin/ptxas
    '';

    pythonImportsCheck = [ "freetoken" ];

    doCheck = false;

    passthru = {
      inherit cudaCapabilities;
    };

    meta = {
      description = "Edge-native MoE serving engine with OpenAI- and Anthropic-compatible APIs";
      homepage = "https://github.com/FlashML-org/FreeToken";
      changelog = "https://github.com/FlashML-org/FreeToken/releases/tag/v${lib.head (lib.splitString "-" finalAttrs.version)}";
      license = lib.licenses.asl20;
      mainProgram = "ft";
      platforms = [ "x86_64-linux" ];
    };
  })

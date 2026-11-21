{
  lib,
  buildGoModule,
  cudaPackages_13,
}:

let
  cudaPackages = cudaPackages_13;
in
buildGoModule {
  pname = "ptxas-fp8-blockscale";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./main.go
      ./rewrite.go
      ./rewrite_test.go
      ./testdata
    ];
  };

  vendorHash = null;

  ldflags = [
    "-s"
    "-w"
  ];

  doInstallCheck = true;

  nativeInstallCheckInputs = [
    cudaPackages.cuda_cuobjdump
    cudaPackages.cuda_nvdisasm
  ];

  installCheckPhase = ''
    runHook preInstallCheck

    PTXAS_FP8_REAL=${lib.getBin cudaPackages.cuda_nvcc}/bin/ptxas \
      $out/bin/ptxas-fp8-blockscale --gpu-name=sm_120a testdata/sm120a.ptx -o fixture.cubin
    cuobjdump -sass fixture.cubin > fixture.sass
    test "$(grep -cF 'QMMA.SF.16832.F32' fixture.sass)" -eq 3
    test "$(grep -cF 'QMMA.16832.F32' fixture.sass)" -eq 0
    test "$(grep -cF 'QMMA.16832.F16' fixture.sass)" -eq 1

    PTXAS_FP8_REAL=${lib.getBin cudaPackages.cuda_nvcc}/bin/ptxas \
      $out/bin/ptxas-fp8-blockscale --version | grep -qF 'ptxas-fp8-blockscale:'

    status=0
    (
      ulimit -u 256
      PTXAS_FP8_REAL=$out/bin/ptxas-fp8-blockscale timeout 30 $out/bin/ptxas-fp8-blockscale --version
    ) || status=$?
    test "$status" -eq 127

    runHook postInstallCheck
  '';

  passthru = { inherit cudaPackages; };

  meta = {
    description = "ptxas wrapper emitting FP8 MMA with FP32 accumulation block-scaled on sm_120";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "ptxas-fp8-blockscale";
  };
}

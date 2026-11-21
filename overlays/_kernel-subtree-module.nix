{
  pname,
  subdir,
  patches,
  postPatch ? "",
  description,
}:
{
  stdenv,
  kernel,
  kernelModuleMakeFlags,
}:
stdenv.mkDerivation {
  inherit pname patches;
  inherit (kernel) version src;

  postPatch = ''
    cd ${subdir}
  ''
  + postPatch;

  nativeBuildInputs = kernel.moduleBuildDependencies;

  enableParallelBuilding = true;

  preBuild = ''
    makeFlagsArray+=("M=$PWD")
  '';

  makeFlags = kernelModuleMakeFlags ++ [
    "-C"
    "${kernel.dev}/lib/modules/${kernel.modDirVersion}/build"
    "INSTALL_MOD_PATH=${placeholder "out"}"
    "INSTALL_MOD_DIR=updates/${baseNameOf subdir}"
  ];

  buildFlags = [ "modules" ];
  installTargets = [ "modules_install" ];

  meta = {
    inherit description;
    inherit (kernel.meta) license;
    platforms = [ "x86_64-linux" ];
  };
}

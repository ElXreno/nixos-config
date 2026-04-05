{
  lib,
  stdenv,
  kernel,
}:

stdenv.mkDerivation {
  pname = "mmio-fan";
  version = "0.1.0";

  src = ./.;

  nativeBuildInputs = kernel.moduleBuildDependencies;

  makeFlags = [
    "KERNEL_SRC=${kernel.dev}/lib/modules/${kernel.modDirVersion}/build"
  ];

  installPhase = ''
    install -D mmio_fan.ko $out/lib/modules/${kernel.modDirVersion}/extra/mmio_fan.ko
  '';

  meta = {
    description = "MMIO GPIO fan thermal cooling device";
    license = lib.licenses.gpl2Only;
    platforms = [ "x86_64-linux" ];
  };
}

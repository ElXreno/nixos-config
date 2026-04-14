_:

_final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_kself: kp: {
      mmio-fan = kp.callPackage (
        {
          lib,
          stdenv,
          kernel,
          kernelModuleMakeFlags,
        }:
        stdenv.mkDerivation {
          pname = "mmio-fan";
          version = "0.1.0";

          src = ./src;

          __structuredAttrs = true;

          nativeBuildInputs = kernel.moduleBuildDependencies;

          enableParallelBuilding = true;

          makeFlags = kernelModuleMakeFlags ++ [
            "KERNEL_SRC=${kernel.dev}/lib/modules/${kernel.modDirVersion}/build"
          ];

          installFlags = [ "INSTALL_MOD_PATH=${placeholder "out"}" ];

          meta = {
            description = "MMIO GPIO fan thermal cooling device";
            license = lib.licenses.gpl2Only;
            platforms = [ "x86_64-linux" ];
          };
        }
      ) { };
    })
  ];
}

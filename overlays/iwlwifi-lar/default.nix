_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      iwlwifi-lar = kp.callPackage (
        {
          stdenv,
          kernel,
          kernelModuleMakeFlags,
          xz,
        }:
        stdenv.mkDerivation {
          pname = "iwlwifi-lar";
          inherit (kernel) version;

          inherit (kernel) src;

          patches = [ ./lar_disable.patch ];

          postPatch = ''
            cd drivers/net/wireless/intel/iwlwifi
            sed -i 's|$(srctree)/||' {d,m}vm/Makefile
          '';

          nativeBuildInputs = kernel.moduleBuildDependencies ++ [ xz ];

          enableParallelBuilding = true;

          preBuild = ''
            makeFlagsArray+=("M=$PWD")
          '';

          makeFlags = kernelModuleMakeFlags ++ [
            "-C"
            "${kernel.dev}/lib/modules/${kernel.modDirVersion}/build"
            "modules"
          ];

          installPhase = ''
            runHook preInstall

            destDir="$out/lib/modules/${kernel.modDirVersion}/updates/iwlwifi"
            mkdir -p "$destDir"
            find . -name '*.ko' -exec cp --parents '{}' "$destDir" \;
            find "$destDir" -name '*.ko' -exec xz -f '{}' \;

            runHook postInstall
          '';

          meta = {
            description = "iwlwifi kernel module rebuilt with the lar_disable modparam";
            inherit (kernel.meta) license;
            platforms = [ "x86_64-linux" ];
          };
        }
      ) { };
    })
  ];
}

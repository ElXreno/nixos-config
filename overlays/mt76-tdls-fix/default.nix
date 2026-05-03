_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      mt76-tdls-fix = kp.callPackage (
        {
          stdenv,
          kernel,
          kernelModuleMakeFlags,
          xz,
        }:
        stdenv.mkDerivation {
          pname = "mt76-tdls-fix";
          inherit (kernel) version;

          inherit (kernel) src;

          patches = [
            ./0001-mt76-mt7925-add-chip-mem-debugfs-for-mcu-sram-dump.patch
            ./0002-mt7925-add-chip_config-debugfs.patch

            (prev.fetchpatch {
              url = "https://github.com/ElXreno/linux/commit/57508f1e6ab29efc4083a7cd71bd86018a55876f.patch";
              hash = "sha256-eBL554a4dGPhYWSZSdLaMcC2FgTo7bkfkXtLWFIjKqY=";
            })
            (prev.fetchpatch {
              url = "https://github.com/ElXreno/linux/commit/cdf4bc267f64c98b496e4b4f71349bcf5f0ad910.patch";
              hash = "sha256-qNdzNLqY1wSPxWRgDwYy+WwE4hXe/XunbP7HKv7GVaI=";
            })
          ];

          postPatch = ''
            cd drivers/net/wireless/mediatek/mt76
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

            destDir="$out/lib/modules/${kernel.modDirVersion}/updates/mt76"
            mkdir -p "$destDir"
            find . -name '*.ko' -exec cp --parents '{}' "$destDir" \;
            find "$destDir" -name '*.ko' -exec xz -f '{}' \;

            runHook postInstall
          '';

          meta = {
            description = "mt76 modules with TDLS direct-link fix for mt7921/mt7922/mt7925 (HW encap offload disabled to bypass broken firmware path)";
            inherit (kernel.meta) license;
            platforms = [ "x86_64-linux" ];
          };
        }
      ) { };
    })
  ];
}

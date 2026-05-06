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
            ./0003-mt7925-add-uni_cmd-debugfs.patch

            (prev.fetchpatch {
              url = "https://github.com/ElXreno/linux/commit/3d910f174bdee76b4b40cb091d4dbe751b9a795a.patch";
              hash = "sha256-RiBngBTdm5VPddK8/DxGNTx8+sIiSGO/Ak6YTtepDQc=";
            })
            (prev.fetchpatch {
              url = "https://github.com/ElXreno/linux/commit/6757598c72e711b647476afa7e0efc567e9ae6f0.patch";
              hash = "sha256-r/V9NblvKDehhmWj/Xz51lg+5Ob35U7rY16UNM5uBNY=";
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

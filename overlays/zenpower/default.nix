_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (kself: kp: {
      zenpower = kp.zenpower.overrideAttrs (old: {
        patches = (old.patches or [ ]) ++ [
          # https://github.com/mattkeenan/zenpower5/pull/8
          (prev.fetchpatch {
            url = "https://github.com/mattkeenan/zenpower5/commit/0ba5635620082047f61bc9663de2d4dd5fc98c56.patch";
            hash = "sha256-m5G/4vg2/9uPpR2DclvvGq3rZ2u01qYymNVrbMshrgw=";
          })
        ];
        makeFlags = (old.makeFlags or [ ]) ++ kself.kernelModuleMakeFlags;
      });
    })
  ];
}

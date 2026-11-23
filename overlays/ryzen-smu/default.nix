_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (kself: kp: {
      ryzen-smu = kp.ryzen-smu.overrideAttrs (old: {
        makeFlags = (old.makeFlags or [ ]) ++ kself.kernelModuleMakeFlags;
      });
    })
  ];
}

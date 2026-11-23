_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      msi-ec = kp.msi-ec.overrideAttrs (old: {
        patches = (old.patches or [ ]) ++ [
          (prev.fetchpatch {
            url = "https://github.com/sidneib-accurati/msi-ec/commit/972f5a5f0c4ce945b21eec6f97f7935fb32aab93.patch";
            hash = "sha256-YRAFRRnZsBDay/55GwDxKvc5/vZ+sAzRw1Sxb+ejoWQ=";
          })
        ];
      });
    })
  ];
}

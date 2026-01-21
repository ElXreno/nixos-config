_:

_final: prev: {
  anyrun = prev.anyrun.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      (prev.fetchpatch {
        url = "https://github.com/anyrun-org/anyrun/pull/304.patch";
        hash = "sha256-r+3komn4H4jZEm3tx1ubbke2LEan1pi99XxeCc1/uUQ=";
      })
    ];
  });
}

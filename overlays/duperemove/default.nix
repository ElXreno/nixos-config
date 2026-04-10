_:

_final: prev: {
  duperemove = prev.duperemove.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      (prev.fetchpatch {
        url = "https://github.com/markfasheh/duperemove/pull/399.patch";
        hash = "sha256-a4MbIZFIFVTfZcs03jTAe5lq76P8FIdrI6xncojI5A4=";
      })
      (prev.fetchpatch {
        url = "https://github.com/markfasheh/duperemove/pull/402.patch";
        hash = "sha256-gR7311AnKxNqqCpaD1k3OvpqibEfhUlWlwAyGH2DlII=";
      })
    ];
  });
}

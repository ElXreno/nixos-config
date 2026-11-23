_:

final: prev: {
  noctalia =
    (prev.noctalia.override { stdenv = final.ccacheTools.wrapStdenv prev.stdenv; }).overrideAttrs
      (prevAttrs: {
        patches = (prevAttrs.patches or [ ]) ++ [
          ./zfs-arc-lru.patch
        ];
      });
}

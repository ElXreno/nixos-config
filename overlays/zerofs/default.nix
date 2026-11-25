_:

_final: prev: {
  zerofs = prev.zerofs.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      ./retry-backoff-60s.patch
      ./export-unflushed-bytes.patch
      ./metadata-memory-size.patch
    ];
  });
}

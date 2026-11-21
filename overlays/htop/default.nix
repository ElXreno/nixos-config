_:

_final: prev: {
  htop = prev.htop.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      ./sm7150-cpu-temperatures.patch
      ./zfs-arc-lru.patch
    ];
  });
}

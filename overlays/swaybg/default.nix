_:

_final: prev: {
  swaybg = prev.swaybg.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [ ./namespace.patch ];
  });
}

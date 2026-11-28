_:

_final: prev: {
  pahole-merged-cu = prev.pahole.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [ ./merged-cu-hashbits.patch ];
  });
}

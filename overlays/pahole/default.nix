_:

_final: prev: {
  pahole = prev.pahole.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [ ./merged-cu-hashbits.patch ];
  });
}

_:

_final: prev: {
  wtype = prev.wtype.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [ ./kde-fake-input.patch ];
  });
}

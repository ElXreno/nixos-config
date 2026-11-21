_:

_final: prev: {
  kopia = prev.kopia.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [ ./s3-idle-conns.patch ];
  });
}

_:

_final: prev: {
  supergfxctl = prev.supergfxctl.overrideAttrs (
    _finalAttrs: previousAttrs: {
      postPatch = (previousAttrs.postPatch or "") + ''
        sed -i "s|/usr/bin/lsof|${prev.lsof}/bin/lsof|" src/lib.rs
      '';
    }
  );
}

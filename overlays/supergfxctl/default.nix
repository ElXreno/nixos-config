_:

_final: prev: {
  supergfxctl = prev.supergfxctl.overrideAttrs (
    _finalAttrs: previousAttrs: {
      postPatch = (previousAttrs.postPatch or "") + ''
        substituteInPlace src/lib.rs \
          --replace-fail '"/usr/bin/lsof"' '"${prev.lsof}/bin/lsof"' \
          --replace-fail 'Command::new("lsof")' 'Command::new("${prev.lsof}/bin/lsof")'
      '';
    }
  );
}

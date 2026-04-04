_:

_final: prev: {
  tailscale = prev.tailscale.overrideAttrs (
    _finalAttrs: prevAttrs: {
      postPatch = (prevAttrs.postPatch or "") + ''
        substituteInPlace wgengine/magicsock/magicsock.go \
          --replace-fail 'socketBufferSize = 7 << 20' 'socketBufferSize = 16 << 20'
      '';
    }
  );
}

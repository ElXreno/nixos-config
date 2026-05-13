_:

_final: prev: {
  tailscale = prev.tailscale.overrideAttrs (
    _finalAttrs: prevAttrs: {
      postPatch = (prevAttrs.postPatch or "") + ''
        substituteInPlace wgengine/magicsock/magicsock.go \
          --replace-fail 'socketBufferSize = 7 << 20' 'socketBufferSize = 16 << 20'

        # https://github.com/tailscale/tailscale/issues/19730
        substituteInPlace net/dns/dns_clone.go \
          --replace-fail $'for k, sv := range src.Routes {\n\t\t\tif sv == nil {\n\t\t\t\tcontinue\n\t\t\t}' \
                         $'for k, sv := range src.Routes {\n\t\t\tif sv == nil {\n\t\t\t\tdst.Routes[k] = nil\n\t\t\t\tcontinue\n\t\t\t}'
      '';
    }
  );
}

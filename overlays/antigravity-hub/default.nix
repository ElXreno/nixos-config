_:

_final: prev:
let
  python = prev.python3;
in
{
  antigravity-hub = prev.antigravity-hub.overrideAttrs (prevAttrs: {
    nativeBuildInputs = (prevAttrs.nativeBuildInputs or [ ]) ++ [ python ];
    postInstall = (prevAttrs.postInstall or "") + ''
      ${python}/bin/python3 ${../_antigravity-gates.py} hub $out/share/antigravity/resources/bin/language_server
    '';
  });
}

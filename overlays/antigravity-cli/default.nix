_:

_final: prev:
let
  python = prev.python3;
in
{
  antigravity-cli = prev.antigravity-cli.overrideAttrs (prevAttrs: {
    nativeBuildInputs = (prevAttrs.nativeBuildInputs or [ ]) ++ [ python ];
    postInstall = (prevAttrs.postInstall or "") + ''
      ${python}/bin/python3 ${../_antigravity-gates.py} agy $out/bin/agy
    '';
  });
}

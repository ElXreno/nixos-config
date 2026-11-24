_: _final: prev:
let
  version = "6.10.1";
in
{
  rea =
    if prev.lib.versionAtLeast prev.rea.version version then
      prev.rea
    else
      prev.rea.overrideAttrs (
        finalAttrs: _prevAttrs: {
          inherit version;

          src = prev.fetchFromGitHub {
            owner = "morluto";
            repo = "rea";
            tag = "rea-agents-${version}";
            hash = "sha256-2ulhZ2GXe1xOHKLNuJo9mTivy4OTiWP9VLHEXnpn5YI=";
          };

          npmDeps = prev.fetchNpmDeps {
            inherit (finalAttrs) src postPatch;
            name = "rea-${version}-npm-deps";
            hash = "sha256-cXQSwTmmu0sDcnFjpmdmaj01PU6gB0UZ+kp0T8EHKH4=";
          };
        }
      );
}

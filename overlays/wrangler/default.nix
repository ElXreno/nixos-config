_: _final: prev:
let
  version = "4.166.0";
in
{
  wrangler =
    if prev.lib.versionAtLeast prev.wrangler.version version then
      prev.wrangler
    else
      prev.wrangler.overrideAttrs (
        finalAttrs: prevAttrs: {
          inherit version;

          installPhase =
            builtins.replaceStrings [ "--prod deploy $out/lib" ] [ "--prod deploy --ignore-scripts $out/lib" ]
              prevAttrs.installPhase;

          src = prev.fetchFromGitHub {
            owner = "cloudflare";
            repo = "workers-sdk";
            rev = "wrangler@${version}";
            hash = "sha256-hmlZDHiGATiEk/jAlf5C3I3aI1IbFRcRPitYOz/2y5I=";
          };

          pnpmDeps = prev.fetchPnpmDeps {
            inherit (finalAttrs)
              pname
              version
              src
              postPatch
              ;
            pnpm = prev.pnpm_10;
            fetcherVersion = 3;
            hash = "sha256-i461Et+53woGt9bT5+/5QPQP1gqp3p9FmGEq+iqiQgM=";
          };
        }
      );
}

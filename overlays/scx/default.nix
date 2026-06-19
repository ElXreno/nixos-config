_:

_final: prev:
let
  rev = "f5e46f7fe7d7a804c422d8e981c5825402d8dac6";
  version = "unstable-2026-06-17";
  src = prev.fetchFromGitHub {
    owner = "sched-ext";
    repo = "scx";
    inherit rev;
    hash = "sha256-NS66OGbDHi0Dx0m4zAoGNueKclPeCN7E/aXNj4sm8YQ=";
  };
in
{
  scx = prev.scx // {
    rustscheds = prev.scx.rustscheds.overrideAttrs (oldAttrs: {
      inherit version src;
      cargoDeps = prev.rustPlatform.fetchCargoVendor {
        inherit (oldAttrs) pname;
        inherit version src;
        hash = "sha256-f3wRxyelxJcrgWW0S680EU5ZPrtjJo3K5NKewOjkqOE=";
      };
      doInstallCheck = false;
    });
  };
}

{ ... }:

_final: prev: {
  gamescope = prev.gamescope.overrideAttrs (prevAttrs: {
    src = prev.fetchFromGitHub {
      owner = "ValveSoftware";
      repo = "gamescope";
      rev = "221394fedaed213f9bce6d18f60242e3120b661f";
      fetchSubmodules = true;
      hash = "sha256-0Kg3J35I3zj8KYw+to51VBDo4Zb19pI6ISrseBOwO1k=";
    };
  });
}

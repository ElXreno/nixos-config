_:

_final: prev: {
  gamescope = prev.gamescope.overrideAttrs (prevAttrs: {
    src = prev.fetchFromGitHub {
      owner = "ValveSoftware";
      repo = "gamescope";
      rev = "221394fedaed213f9bce6d18f60242e3120b661f";
      fetchSubmodules = true;
      hash = "sha256-0Kg3J35I3zj8KYw+to51VBDo4Zb19pI6ISrseBOwO1k=";
    };

    patches = (prevAttrs.patches or [ ]) ++ [
      (prev.fetchpatch {
        url = "https://github.com/zlice/gamescope/commit/fa900b0694ffc8b835b91ef47a96ed90ac94823b.patch";
        hash = "sha256-eIHhgonP6YtSqvZx2B98PT1Ej4/o0pdU+4ubdiBgBM4=";
      })
    ];
  });
}

_:

_final: prev: {
  ncps = prev.ncps.overrideAttrs (prevAttrs: {
    src = prev.fetchFromGitHub {
      owner = "kalbasit";
      repo = "ncps";
      rev = "11ca44edcee00db5f12c502c071206d792db6d32";
      hash = "sha256-HqqMIVKFgFV/0Bx/FpP1r7a+wlBDLi2QLS8r1u6cVww=";
    };
    vendorHash = "sha256-iy4r5t9xOXu6QluTHYXOjTmI8Tx5Gs/H7GZW7OIyyaI=";
  });
}

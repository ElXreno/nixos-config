_:

_final: prev: {
  linux-firmware = prev.linux-firmware.overrideAttrs (_: {
    version = "20260423-unstable-5b50ca02";
    src = prev.fetchFromGitLab {
      owner = "kernel-firmware";
      repo = "linux-firmware";
      rev = "5b50ca02f586e0e44820d1b98d6c7fabc465b2d2";
      hash = "sha256-zAVetI1evTaoN68gntp7HrEeufz5RtbRn6Zgef8WyPE=";
    };
  });
}

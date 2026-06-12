_:

_final: prev: {
  linux-firmware = prev.linux-firmware.overrideAttrs (
    _finalAttrs: _prevAttrs: {
      version = "20260519-unstable-2026-06-05";
      src = prev.fetchFromGitLab {
        owner = "kernel-firmware";
        repo = "linux-firmware";
        rev = "4d575d919be3180ced75c600b7c233ecd66ea340";
        hash = "sha256-DW9huBGWCl+TprAoCgrhc32eorA8h93ko9o4apTWI5Y=";
      };
    }
  );
}

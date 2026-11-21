_:

_final: prev: {
  wvkbd = prev.wvkbd.overrideAttrs (prevAttrs: {
    version = "0.20-unstable-2026-09-29";

    src = prev.fetchFromGitHub {
      owner = "jjsullivan5196";
      repo = "wvkbd";
      rev = "bbecd996aae5a4dc4bc32bd643d37cefa9f63752";
      hash = "sha256-F1sYakGsFkJI5FXWTEJIeXrV4QP6WMh+eg7y2hHgxKk=";
    };

    patches = (prevAttrs.patches or [ ]) ++ [
      ./show-reentrancy.patch
      ./pc-layouts.patch
    ];
  });
}

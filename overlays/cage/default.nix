_:

_final: prev: {
  cage = prev.cage.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      (prev.fetchpatch {
        url = "https://github.com/cage-kiosk/cage/commit/e84cd4faae1ff0253d6dab82adcf9d2fac4c89b7.patch";
        hash = "sha256-2U07HFHQ3pE7I+8mW4vm2yWQIS8Q2LqTb4d4C5keqdQ=";
      })
    ];
  });
}

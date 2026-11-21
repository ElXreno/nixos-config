_:

_final: prev: {
  nix-update = prev.nix-update.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      # workaround for https://github.com/Mic92/nix-update/issues/693
      ./prefetch-dirty-flake.patch
    ];
  });
}

_:

final: prev: {
  nix-update = prev.nix-update.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      # Mic92/nix-update#692
      (final.fetchpatch {
        url = "https://github.com/Mic92/nix-update/commit/d545c1fad667e573128cad0db55fdc77134a4d3c.patch";
        hash = "sha256-CVSihQevWIwKSnjKo9ilw1hdi4a1t0xr6DunAUKSBFA=";
      })
    ];
  });
}

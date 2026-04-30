_:

_final: prev: {
  nixos-facter = prev.nixos-facter.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [
      ./tolerate-unknown-bus.patch
    ];
  });
}

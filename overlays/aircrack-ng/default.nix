_: _final: prev: {
  aircrack-ng = prev.aircrack-ng.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [
      ./0001-osdep-linux-read-the-channel-and-frequency-through-n.patch
    ];
  });
}

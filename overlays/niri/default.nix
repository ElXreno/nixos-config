{ inputs, ... }:

_final: prev: { inherit (inputs.niri.packages.${prev.stdenv.hostPlatform.system}) niri-unstable; }

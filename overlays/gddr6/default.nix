{ inputs, ... }:

_final: prev: { inherit (inputs.gddr6.packages.${prev.stdenv.hostPlatform.system}) gddr6; }

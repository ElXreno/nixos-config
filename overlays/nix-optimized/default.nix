{ inputs, ... }:
_final: prev: {
  inherit (inputs.nix-optimized.packages.${prev.stdenv.hostPlatform.system}) nix-optimized;
}

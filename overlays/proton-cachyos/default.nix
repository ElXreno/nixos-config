{ inputs, ... }:
_final: prev: {
  inherit (inputs.proton-cachyos.packages.${prev.stdenv.hostPlatform.system})
    proton-cachyos-x86_64_v3
    ;
}

{ inputs, ... }:

_final: prev: {
  inherit (inputs.roj.packages.${prev.stdenv.hostPlatform.system}) roj-crawler;
}

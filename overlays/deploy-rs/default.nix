{ inputs, ... }:

_final: prev: { inherit (inputs.deploy-rs.packages.${prev.stdenv.hostPlatform.system}) deploy-rs; }

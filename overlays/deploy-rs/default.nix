{ inputs, ... }:

_final: prev: { inherit (inputs.deploy-rs.packages.${prev.system}) deploy-rs; }

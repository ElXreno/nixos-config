{ inputs, ... }:

_final: prev:
prev.lib.optionalAttrs (prev.config.cudaSupport or false) {
  buildMozillaMach =
    opts:
    (prev.buildMozillaMach opts).override {
      inherit (inputs.nixpkgs.legacyPackages.${prev.stdenv.hostPlatform.system}) onnxruntime;
    };
}

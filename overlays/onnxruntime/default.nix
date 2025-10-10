{ ... }:

_final: prev: {
  onnxruntime = prev.onnxruntime.override {
    cudaSupport = false; # I don't want to rebuild that
  };
}

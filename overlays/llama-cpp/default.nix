_:

final: prev: {
  llama-cpp-rerank = prev.llama-cpp.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [ ./rerank-prefix-reuse.patch ];
    cmakeFlags =
      (prevAttrs.cmakeFlags or [ ])
      ++
        map
          (
            language:
            final.lib.cmakeFeature "CMAKE_${language}_COMPILER_LAUNCHER" "${final.ccacheTools.launcher}"
          )
          [
            "C"
            "CXX"
            "CUDA"
          ];
  });

  llama-cpp-rerank-noblas = final.llama-cpp-rerank.override { blasSupport = false; };
}

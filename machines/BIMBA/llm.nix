{
  config,
  namespace,
  pkgs,
  ...
}:
let
  cfg = config.${namespace}.services.llama-cpp;
  threads = [
    "-t"
    "8"
    "-tb"
    "8"
  ];
in
{
  ${namespace}.services.llama-cpp = {
    enable = true;
    package = pkgs.llama-cpp-rerank-noblas;
    instances.memory = {
      model = "${cfg.stateDir}/models/v5-small-retrieval-F16.gguf";
      port = 8096;
      backendPort = 18096;
      idleTimeout = "30min";
      extraArgs = [
        "--embedding"
        "--pooling"
        "last"
        "-np"
        "1"
        "-c"
        "8192"
        "-b"
        "8192"
        "-ub"
        "8192"
      ]
      ++ threads;
    };
    instances.rerank = {
      model = "${cfg.stateDir}/models/Qwen3-Reranker-0.6B-BF16.gguf";
      port = 8101;
      backendPort = 18101;
      idleTimeout = "30min";
      extraArgs = [
        "--reranking"
        "-np"
        "1"
        "-c"
        "8192"
        "-b"
        "2048"
        "-ub"
        "2048"
      ]
      ++ threads;
    };
  };
}

{
  config,
  namespace,
  pkgs,
  ...
}:
let
  services = config.${namespace}.services;
  swiftGguf = "/mnt/models/ukisai--Swift-1.5-Qwen3.8-Flash-Next-GSQ-RCO/Swift-Qwen3.8-Flash-Next-GSQ-RCO-IQ3_S-00001-of-00002.gguf";
in
{
  ${namespace} = {
    system.hardware.gpu.nvidia.cudaCapability = "12.0";

    services = {
      tei = {
        enable = true;
        instances.embed = {
          model = "BAAI/bge-m3";
          port = 8095;
          backendPort = 18095;
          prometheusPort = 9105;
          extraArgs = [
            "--max-client-batch-size"
            "128"
          ];
        };
      };

      freetoken = {
        enable = true;
        instances.ornith = {
          model = "/mnt/models/ornith-1.5-122b-nvfp4-ftw-b12x-mtp";
          servedModelName = "ornith-1.5-nvfp4";
          memoryMax = "84G";
          port = 8097;
          backendPort = 18097;
          ctxSize = 262144;
          kvCacheDtype = "fp8_e4m3";
          linearStateCacheRatio = 4.0;
          maxPrefillLength = 4096;
          maxRunningRequests = 2;
          minP = 0.05;
          moeCpuThreads = 8;
          moeHybridFetchFraction = 0.25;
          moePrefillHitD2d = true;
          moeStrategy = "hybrid";
          quantBackend = "moe.nvfp4=b12x,linear.nvfp4=triton";
          decodeStepsPerPrefillChunk = 64;
          prefixCache = {
            enable = true;
            chunkSnapshots = true;
          };
          embedTableHost = true;
          mtpVerify = true;
          enableCacheReport = true;
          ablation = {
            enable = true;
            direction = "/mnt/models/refusal-directions/ornith-1.5-122b-atomic.safetensors";
            mode = "writers";
            alpha = 1.25;
          };
          environment = {
            FREETOKEN_B12X_INACTIVE_SLOTS = "1";
            FREETOKEN_CPU_MOE_PREFETCH = "load";
            FREETOKEN_DEMAND_COPY = "ce";
            FREETOKEN_HYBRID_SPLIT_GEMM = "copy";
            FREETOKEN_KV_LEND = "1";
            FREETOKEN_KV_LEND_GDN = "1";
            FREETOKEN_SPEC_GATE = "0.45";
          };
        };
        instances.ornith35 = {
          model = "/mnt/models/ornith-1.5-35b-nvfp4-ftw-b12x-mtp";
          servedModelName = "ornith-1.5-35b-nvfp4";
          port = 8098;
          backendPort = 18098;
          ctxSize = 262144;
          kvCacheDtype = "fp8_e4m3";
          linearStateCacheRatio = 4.0;
          maxPrefillLength = 4096;
          maxRunningRequests = 2;
          memoryRatio = 0.7;
          minP = 0.05;
          moeCpuThreads = 8;
          moePrefillHitD2d = true;
          quantBackend = "moe.nvfp4=b12x,linear.nvfp4=triton";
          decodeStepsPerPrefillChunk = 64;
          prefixCache = {
            enable = true;
            chunkSnapshots = true;
          };
          embedTableHost = true;
          mtpVerify = true;
          enableCacheReport = true;
          environment = {
            FREETOKEN_KV_LEND = "1";
            FREETOKEN_KV_LEND_GDN = "1";
          };
        };
      };

      searxng.enable = true;

      llm-gateway.enable = true;

      strata = {
        enable = true;
        conversationCacheDir = "/mnt/models/strata-conversations";
        instances.qwen = {
          pack = "/mnt/models/qwen3.8-flash-next-nvfp4-gptq-strata/pack";
          native = "/mnt/models/qwen3.8-flash-next-nvfp4-gptq-strata/qwen-nvfp4-gptq-dense.gguf";
          nativeDenseGguf = "/mnt/models/qwen3.8-flash-next-nvfp4-gptq-strata/qwen-nvfp4-gptq-dense.gguf";
          pleGguf = "/mnt/models/qwen3.8-flash-next-nvfp4-gptq-strata/ple-fp8.gguf";
          embdGguf = "/mnt/models/qwen3.8-flash-next-nvfp4-gptq-strata/token-embd-bf16.gguf";
          mtp = "/mnt/models/qwen3.8-flash-next-nvfp4-gptq-strata/mtp";
          port = 8099;
          backendPort = 18099;
          ctxSize = 262144;
          kv = "int8";
          kvResident = 32768;
          conversationCache.minFreeMib = 0;
          ablation = {
            enable = true;
            direction = "${pkgs.fetchurl {
              url = "https://raw.githubusercontent.com/sergqwer/strata-nvfp4/6c026a0e73f763c09cb7cc5af824bec5aff2df3f/data/uncensor/qwen-nvfp4-gptq-uncensored.gguf";
              hash = "sha256-ouw+mZckAipV1I0wgivRCxslkWzMIhAToRmmdNBqRrU=";
            }}";
            firstLayer = 8;
            lastLayer = 33;
          };
          webSearch.searxng = "http://127.0.0.1:${toString config.${namespace}.ports.searxng}";
          memoryMax = "84G";
          environment = {
            STRATA_EXCHANGE_ROTATE = "1";
            STRATA_POOL_UMWAIT = "50000";
            STRATA_PREFILL_ROUTED_STREAM = "1";
            STRATA_SELECT_TWO_PASS = "1";
          };
          extraArgs = [
            "--adapt-async"
            "1"
            "--lookup-max-t"
            "16"
            "--prompt-cache"
            "10"
            "--prompt-cache-spread"
            "--resident-budget-gib"
            "48"
          ];
        };
        instances.swift = {
          pack = "/mnt/models/swift-1.5-gsq-strata/pack";
          native = swiftGguf;
          pleGguf = swiftGguf;
          servedModelName = "swift-1.5";
          port = 8100;
          backendPort = 18100;
          conversationCache.minFreeMib = 0;
          inherit (services.strata.instances.qwen)
            mtp
            ctxSize
            kv
            kvResident
            webSearch
            memoryMax
            environment
            extraArgs
            ;
        };
      };

      xolotl = {
        enable = true;
        cacheDir = "/mnt/models/xolotl-cache";
        instances.deepseek =
          let
            root = "/mnt/models/deepseek-v4.1-flash-1017-xolotl";
            fast = "${root}/xq/v4-fast/V41-XQ-1017-v4.mxfp4-q3k.q2k.im.x41s";
          in
          {
            model = "${root}/xq/v4/V41-XQ-1017-v4.gguf";
            rank = "${root}/xq/v4/rank-1017-v4.bin";
            engram = [
              "${root}/engram-a"
              "${root}/engram-b"
            ];
            tokenizer = "${root}/xq/tokenizer";
            dspark = "${root}/xq/V41-MTP-Vision-1017.Q2_K.gguf";
            shadow = [
              "${fast}.g10d42"
              "${fast}.g42d10"
              fast
              "${fast}.g10d143"
              "${fast}.g143d10"
            ];
            port = 8101;
            backendPort = 18101;
            ctxSize = 524288;
            environment.STRATA_ZFS_ARC = "1";
            webSearch.searxng = "http://127.0.0.1:${toString config.${namespace}.ports.searxng}";
          };
      };
    };
  };
}

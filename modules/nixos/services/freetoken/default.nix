{
  config,
  namespace,
  lib,
  pkgs,
  ...
}@args:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    escapeShellArg
    escapeShellArgs
    optionalString
    types
    ;
  cfg = config.${namespace}.services.freetoken;
  freetoken = pkgs.${namespace}.freetoken.override {
    cudaCapabilities = [ config.${namespace}.system.hardware.gpu.nvidia.cudaCapability ];
  };
  idle = import ../_idle-backend.nix args;

  environment = {
    HOME = cfg.stateDir;
    XDG_CACHE_HOME = "${cfg.stateDir}/cache";
    HF_HOME = "${cfg.stateDir}/hf";
    FLASHINFER_WORKSPACE_BASE = cfg.stateDir;
    TRITON_CACHE_DIR = "${cfg.stateDir}/cache/triton";
    TVM_FFI_CUDA_ARCH_LIST = config.${namespace}.system.hardware.gpu.nvidia.cudaCapability;
  };

  benchProfile = pkgs.writeShellScript "freetoken-bench-profile" ''
    stamp="$XDG_CACHE_HOME/freetoken/benchbw.stamp"
    key=$(printf '%s\n' "${freetoken}" "$PATH" "$TVM_FFI_CUDA_ARCH_LIST" | sha256sum | cut -c1-64)
    [ "$(cat "$stamp" 2>/dev/null)" = "$key" ] && exit 0
    ${freetoken}/bin/ft bench bw
    mkdir -p "$(dirname "$stamp")"
    echo "$key" > "$stamp"
  '';

  runner =
    name: i:
    idle.runner {
      name = "freetoken-${name}-run";
      command = lib.concatStringsSep " \\\n  " [
        "${freetoken}/bin/ft serve"
        "--model-path ${escapeShellArg i.model}"
        "--served-model-name ${escapeShellArg i.servedModelName}"
        "--host 127.0.0.1"
        "--port ${toString i.backendPort}"
        "--max-running-requests ${toString i.maxRunningRequests}"
        "--kv-cache-dtype ${i.kvCacheDtype}"
        (optionalString (i.kvCacheScales != null) "--kv-cache-scales ${escapeShellArg i.kvCacheScales}")
        "--kv-reserve-tokens ${toString i.ctxSize}"
        "--linear-state-cache-ratio ${toString i.linearStateCacheRatio}"
        "--moe-strategy ${i.moeStrategy}"
        "--moe-cache-auto"
        (optionalString (i.moeCpuThreads != null) "--moe-cpu-threads ${toString i.moeCpuThreads}")
        (optionalString (
          i.moeHybridFetchFraction != null
        ) "--moe-hybrid-fetch-fraction ${toString i.moeHybridFetchFraction}")
        (optionalString (
          i.moeHybridSmallPrefill > 0
        ) "--moe-hybrid-small-prefill ${toString i.moeHybridSmallPrefill}")
        (optionalString i.moePrefillHitD2d "--moe-prefill-hit-d2d")
        (optionalString (i.quantBackend != null) "--quant-backend ${escapeShellArg i.quantBackend}")
        "--decode-steps-per-prefill-chunk ${toString i.decodeStepsPerPrefillChunk}"
        (optionalString i.prefixCache.enable (
          "--prefix-cache-dir ${escapeShellArg "${cfg.stateDir}/cache/${name}"} "
          + "--prefix-cache-host-bytes ${toString i.prefixCache.hostBytes} "
          + "--prefix-cache-flush-idle-seconds ${toString i.prefixCache.flushIdleSeconds}"
          + optionalString i.prefixCache.chunkSnapshots " --prefix-cache-chunk-snapshots"
        ))
        "--memory-ratio ${toString i.memoryRatio}"
        "--max-prefill-length ${toString i.maxPrefillLength}"
        (optionalString (i.minP != null) "--default-min-p ${toString i.minP}")
        "--reasoning-parser ${i.reasoningParser}"
        "--tool-call-parser ${i.toolCallParser}"
        (optionalString i.textModelOnly "--text-model-only")
        (optionalString i.enableCacheReport "--enable-cache-report")
        (optionalString i.moeCollectStats "--moe-collect-stats")
        (optionalString i.embedTableHost "--embed-weights host")
        (optionalString i.mtpVerify "--mtp-verify")
        (optionalString i.ablation.enable (
          "--ablate-direction ${escapeShellArg "${i.ablation.direction}"} "
          + "--ablate-layer ${toString i.ablation.layer} "
          + "--ablate-alpha ${toString i.ablation.alpha}"
          + optionalString (i.ablation.mode != "stream") " --ablate-mode ${i.ablation.mode}"
        ))
        (escapeShellArgs i.extraArgs)
      ];
      healthCheck = "${pkgs.curl}/bin/curl -sf -m 2 \"http://127.0.0.1:${toString i.backendPort}/health\" 2>/dev/null \\\n    | ${pkgs.gnugrep}/bin/grep -q '\"status\": *\"ok\"'";
      interval = "1";
    };

  instanceOpts = types.submodule (
    { config, ... }:
    {
      options = idle.instanceOptions { idleTimeout = "1h"; } // {
        model = mkOption {
          type = types.str;
          description = "Absolute path to the local model directory (HF layout) to serve.";
        };
        servedModelName = mkOption {
          type = types.str;
          default = baseNameOf config.model;
          description = "Model id advertised on the OpenAI-compatible API.";
        };
        ctxSize = mkOption {
          type = types.int;
          default = 32768;
          description = "KV tokens reserved for the paged cache; the context window clients get.";
        };
        kvCacheDtype = mkOption {
          type = types.enum [
            "auto"
            "fp8_e4m3"
          ];
          default = "auto";
          description = "Paged KV storage dtype; fp8_e4m3 halves the KV bytes per token.";
        };
        kvCacheScales = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Per-layer fp8 KV dequant scales JSON; needs kvCacheDtype = fp8_e4m3.";
        };
        linearStateCacheRatio = mkOption {
          type = types.float;
          default = 2.0;
          description = "GDN snapshot cache slots per running request (floor 4); one slot per resumable cached prefix.";
        };
        moeStrategy = mkOption {
          type = types.str;
          default = "auto";
          description = "Expert placement: auto, offload, hybrid, cpu or fused.";
        };
        moeCpuThreads = mkOption {
          type = types.nullOr types.int;
          default = null;
          description = "Worker threads of the CPU expert executor; null lets the engine pick.";
        };
        moeHybridFetchFraction = mkOption {
          type = types.nullOr types.float;
          default = null;
          description = "Share of each hybrid decode step's expert misses fetched over PCIe; null uses the benched bandwidth ratio.";
        };
        moeHybridSmallPrefill = mkOption {
          type = types.int;
          default = 0;
          description = "Prefill chunks of at most this many tokens run on the hybrid decode path instead of streaming every expert; 0 disables.";
        };
        moePrefillHitD2d = mkEnableOption "copying cache-resident experts device-side during prefill, streaming only the misses";
        quantBackend = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Kernel per quantized layer kind, e.g. moe.nvfp4=marlin.";
        };
        decodeStepsPerPrefillChunk = mkOption {
          type = types.int;
          default = 0;
          description = "Decode steps granted to running requests per prefill chunk of another request; 0 is strict prefill priority.";
        };
        prefixCache = {
          enable = mkEnableOption "keeping the prefix cache in a host arena across restarts" // {
            default = true;
          };
          hostBytes = mkOption {
            type = types.int;
            default = 34359738368;
            description = "Arena capacity in bytes.";
          };
          flushIdleSeconds = mkOption {
            type = types.int;
            default = 600;
            description = "Idle time after which unsaved tree metadata is written.";
          };
          chunkSnapshots = mkEnableOption "committing the GDN state at every prefill chunk boundary" // {
            default = true;
          };
        };
        memoryRatio = mkOption {
          type = types.float;
          default = 0.9;
          description = "Fraction of free VRAM the engine may use.";
        };
        maxPrefillLength = mkOption {
          type = types.int;
          default = 2048;
          description = "Prefill chunk in tokens; the activation peak scales with it.";
        };
        maxRunningRequests = mkOption {
          type = types.int;
          default = 1;
          description = "Concurrent decode slots.";
        };
        minP = mkOption {
          type = types.nullOr types.float;
          default = null;
          description = "min_p for requests that set none: tokens below this share of the most likely one are never sampled.";
        };
        textModelOnly = mkOption {
          type = types.bool;
          default = true;
          description = "Skip the vision towers of multimodal checkpoints.";
        };
        reasoningParser = mkOption {
          type = types.str;
          default = "auto";
          description = "Parser splitting chain-of-thought into reasoning_content.";
        };
        toolCallParser = mkOption {
          type = types.str;
          default = "auto";
          description = "Tool-call format parser.";
        };
        enableCacheReport = mkOption {
          type = types.bool;
          default = false;
          description = "Report the prefix-cache hit of each response in its usage block.";
        };
        moeCollectStats = mkOption {
          type = types.bool;
          default = false;
          description = "Count expert-cache hits, PCIe fetches and CPU misses per decode step.";
        };
        embedTableHost = mkOption {
          type = types.bool;
          default = false;
          description = "Keep the input embedding table in pinned host memory and give its VRAM to the caches.";
        };
        mtpVerify = mkOption {
          type = types.bool;
          default = false;
          description = "Draft one token ahead with the MTP head and commit it when the step confirms it.";
        };
        ablation = {
          enable = mkEnableOption "removing a direction from the residual stream";
          direction = mkOption {
            type = types.str;
            description = "Absolute path to a safetensors file with a `direction` tensor of shape [layers, hidden].";
          };
          mode = mkOption {
            type = types.enum [
              "stream"
              "writers"
            ];
            default = "stream";
            description = "`stream` cuts row `layer` from the residual stream at every layer and in the MTP head; `writers` cuts row l from what layer l writes, the runtime form of a per-layer weight orthogonalization.";
          };
          layer = mkOption {
            type = types.int;
            default = 0;
            description = "Row of the direction table to remove; ignored in the `writers` mode.";
          };
          alpha = mkOption {
            type = types.float;
            default = 1.0;
            description = "Share of the component removed at each cut.";
          };
        };
        environment = mkOption {
          type = types.attrsOf types.str;
          default = { };
          description = "Engine tuning variables (FREETOKEN_*).";
        };
        memoryMax = mkOption {
          type = types.str;
          default = "40G";
          description = "Hard cgroup memory cap for the backend.";
        };
      };
    }
  );
in
{
  options.${namespace}.services.freetoken = {
    enable = mkEnableOption "running FreeToken inference servers";

    stateDir = mkOption {
      type = types.str;
      default = "/var/lib/freetoken";
      description = "State directory holding the JIT kernel caches and the bandwidth profile.";
    };

    instances = mkOption {
      type = types.attrsOf instanceOpts;
      default = { };
    };
  };

  config = mkIf cfg.enable (
    idle.units {
      name = "freetoken";
      label = "FreeToken";
      inherit (cfg) stateDir instances;
      describe = i: i.servedModelName;
      extraAddressFamilies = [ "AF_NETLINK" ];
      backend = name: i: {
        path = [ pkgs.bash ];
        environment = environment // i.environment;
        serviceConfig = {
          ExecStartPre = benchProfile;
          ExecStart = runner name i;
          TimeoutStartSec = "30min";
          TimeoutStopSec = 60;
          MemoryMax = i.memoryMax;
          OOMScoreAdjust = 500;
        };
      };
    }
  );
}

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
    optionals
    types
    ;
  cfg = config.${namespace}.services.xolotl;
  json = pkgs.formats.json { };
  idle = import ../_idle-backend.nix args;
  mib = n: toString (n * 1024 * 1024);

  serveConfig =
    name: i:
    json.generate "xolotl-${name}.json" (
      lib.recursiveUpdate (
        {
          exe = "${cfg.package}/share/xolotl/tools/v41/serve/ds41-engine.sh";
          args = [
            "-m"
            i.model
            "--backend"
            "cuda"
            "--experts"
            "gpu"
            "--engram"
            (lib.concatStringsSep ":" i.engram)
            "--rank"
            i.rank
            "--ctx"
            (toString i.ctxSize)
            "--stop"
            "1"
            "--arena-adapt"
            "--route-bias"
            (toString i.routeBias)
            "--route-bias-file"
            (toString i.routeBiasFile)
            "--arena-gib"
            (toString i.arenaGib)
            "--prompt-batch"
            (toString i.promptBatch)
          ]
          ++ optionals i.kvFp4 [ "--kv-fp4" ]
          ++ i.extraArgs;
          cwd = cfg.stateDir;
          inherit (i) tokenizer;
          model_name = i.servedModelName;
          log = "${cfg.stateDir}/${name}.log";
          gpu = 0;
          sampling = {
            temperature = 1;
            top_p = 0.95;
          };
          reasoning_budget_tokens = i.reasoningBudget;
          env = {
            STRATA_BUILD_DIR = "${cfg.package}/share/xolotl/build-xolotl";
            STRATA_NO_SCOPE = "1";
            STRATA_ATTN_F16_GATHER = "1";
            DS4_LOCK = "${cfg.stateDir}/${name}.lock";
          }
          // lib.optionalAttrs (i.dspark != null) { STRATA_SERVE_DSPARK = i.dspark; }
          // lib.optionalAttrs (i.shadow != [ ]) { XOLOTL_SHADOW = lib.concatStringsSep ":" i.shadow; }
          // lib.optionalAttrs i.umwaitSync { XOLOTL_UMWAIT_SYNC = "1"; }
          // lib.optionalAttrs (i.denseCacheDir != null) { STRATA_DENSE_REQUANT_CACHE = i.denseCacheDir; }
          // lib.optionalAttrs (i.conversationCache.disk.dir != null) {
            STRATA_PREFIX_CACHE_DIR = i.conversationCache.disk.dir;
            STRATA_PREFIX_CACHE_BYTES = mib i.conversationCache.disk.mib;
            STRATA_PREFIX_CACHE_MIN_FREE_BYTES = mib i.conversationCache.disk.minFreeMib;
            STRATA_PREFIX_CACHE_QUEUE_BYTES = mib i.conversationCache.disk.queueMib;
          }
          // i.environment;
        }
        // lib.optionalAttrs (i.webSearch.searxng != null) {
          web_search = {
            inherit (i.webSearch) searxng;
            max_results = i.webSearch.maxResults;
          };
        }
      ) i.settings
    );

  runner =
    name: i:
    idle.runner {
      name = "xolotl-${name}-run";
      command = lib.concatStringsSep " " [
        "${cfg.package}/bin/xolotl-ds41-serve --engine strata"
        "--config ${serveConfig name i}"
        "--host 127.0.0.1"
        "--port ${toString i.backendPort}"
      ];
      healthCheck = "${pkgs.curl}/bin/curl -sf -m 2 \"http://127.0.0.1:${toString i.backendPort}/health\" 2>/dev/null \\\n    | ${pkgs.gnugrep}/bin/grep -q '\"loaded\": *true'";
      interval = "1";
    };

  instanceOpts = types.submodule (
    { name, ... }:
    {
      options = idle.instanceOptions { idleTimeout = "1h"; } // {
        model = mkOption {
          type = types.str;
          description = "DeepSeek-V4.1 XQ GGUF (-m).";
        };
        rank = mkOption {
          type = types.str;
          description = "Expert rank file that seeds the tiers (--rank).";
        };
        engram = mkOption {
          type = types.nonEmptyListOf types.str;
          description = "Engram table directories (--engram).";
        };
        tokenizer = mkOption {
          type = types.str;
          description = "Directory with the tokenizer files.";
        };
        dspark = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "DSpark MTP drafter GGUF; null decodes without it.";
        };
        shadow = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Fast-mode sidecar files (XOLOTL_SHADOW); empty keeps every expert exact.";
        };
        servedModelName = mkOption {
          type = types.str;
          default = "deepseek-v4.1-flash";
          description = "Model id advertised on the OpenAI and Anthropic APIs.";
        };
        ctxSize = mkOption {
          type = types.int;
          default = 262144;
          description = "Context window in tokens.";
        };
        kvFp4 = mkOption {
          type = types.bool;
          default = true;
          description = "Whether the KV cache is stored in FP4 (--kv-fp4).";
        };
        arenaGib = mkOption {
          type = types.int;
          default = 64;
          description = "Pinned RAM arena for experts that miss VRAM.";
        };
        routeBias = mkOption {
          type = types.float;
          default = 0.5;
          description = "Decode routing bonus for experts already in VRAM; 0 routes exactly.";
        };
        routeBiasFile = mkOption {
          type = types.float;
          default = 0.3;
          description = "Decode routing penalty for experts read from the file tier; 0 routes exactly.";
        };
        promptBatch = mkOption {
          type = types.int;
          default = 12288;
          description = "Prompt tokens read per batch.";
        };
        reasoningBudget = mkOption {
          type = types.int;
          default = 32768;
          description = "Thinking tokens allowed per reply unless a request sets its own.";
        };
        umwaitSync = mkOption {
          type = types.bool;
          default = true;
          description = "Whether CUDA stream waits sleep in UMWAIT instead of spinning (XOLOTL_UMWAIT_SYNC).";
        };
        denseCacheDir = mkOption {
          type = types.nullOr types.str;
          default = "${cfg.cacheDir}/${name}/dense";
          defaultText = "\${cacheDir}/<instance>/dense";
          description = "Directory keeping the requantized dense weights between loads; null keeps the engine default next to the model.";
        };
        conversationCache.disk = {
          dir = mkOption {
            type = types.nullOr types.str;
            default = "${cfg.cacheDir}/${name}/prefix";
            defaultText = "\${cacheDir}/<instance>/prefix";
            description = "Directory of the persistent prompt-prefix cache; null disables it.";
          };
          mib = mkOption {
            type = types.ints.positive;
            default = 131072;
            description = "Disk budget of the prefix cache.";
          };
          minFreeMib = mkOption {
            type = types.ints.positive;
            default = 51200;
            description = "Free space the prefix cache leaves on its filesystem.";
          };
          queueMib = mkOption {
            type = types.ints.positive;
            default = 2048;
            description = "RAM held by prefix-cache writes that are not on disk yet.";
          };
        };
        settings = mkOption {
          inherit (json) type;
          default = { };
          description = "Keys merged over the generated serve config.";
        };
        environment = mkOption {
          type = types.attrsOf types.str;
          default = { };
          description = "Engine tuning variables (STRATA_*, XOLOTL_*).";
        };
        memoryMax = mkOption {
          type = types.str;
          default = "88G";
          description = "Hard cgroup memory cap for the backend.";
        };
        webSearch = {
          searxng = mkOption {
            type = types.nullOr types.str;
            default = null;
            example = "http://127.0.0.1:8888";
            description = "SearXNG instance that runs Anthropic web_search server tool calls; null leaves them to the client.";
          };
          maxResults = mkOption {
            type = types.ints.positive;
            default = 10;
            description = "Results given to the model per search.";
          };
        };
      };
    }
  );
in
{
  options.${namespace}.services.xolotl = {
    enable = mkEnableOption "running Xolotl DeepSeek-V4.1 inference servers";

    package = mkOption {
      type = types.package;
      default = pkgs.${namespace}.xolotl;
      defaultText = "pkgs.\${namespace}.xolotl";
      description = "Xolotl build to run.";
    };

    stateDir = mkOption {
      type = types.str;
      default = "/var/lib/xolotl";
      description = "State directory holding the engine logs and locks.";
    };

    cacheDir = mkOption {
      type = types.str;
      default = "${cfg.stateDir}/cache";
      defaultText = "\${stateDir}/cache";
      description = "Parent of the per-instance prefix and dense-weight caches.";
    };

    instances = mkOption {
      type = types.attrsOf instanceOpts;
      default = { };
    };
  };

  config = mkIf cfg.enable (
    let
      cacheDirs =
        i:
        lib.filter (d: d != null) [
          i.conversationCache.disk.dir
          i.denseCacheDir
        ];
      withCache = lib.filterAttrs (_: i: cacheDirs i != [ ]) cfg.instances;
    in
    lib.mkMerge [
      (idle.units {
        name = "xolotl";
        label = "Xolotl";
        inherit (cfg) stateDir instances;
        describe = i: i.servedModelName;
        backend = name: i: {
          requires = lib.optional (cacheDirs i != [ ]) "xolotl-${name}-cache-dir.service";
          after = lib.optional (cacheDirs i != [ ]) "xolotl-${name}-cache-dir.service";
          path = with pkgs; [
            bash
            coreutils
            gnugrep
            iproute2
            util-linux
          ];
          environment.HOME = cfg.stateDir;
          serviceConfig = {
            ExecStart = runner name i;
            ReadWritePaths = [ cfg.stateDir ] ++ cacheDirs i;
            TimeoutStartSec = "10min";
            TimeoutStopSec = 90;
            KillMode = "mixed";
            LimitMEMLOCK = "infinity";
            MemoryMax = i.memoryMax;
            OOMScoreAdjust = 500;
          };
        };
      })
      {
        systemd.services = lib.mapAttrs' (
          name: i:
          lib.nameValuePair "xolotl-${name}-cache-dir" {
            description = "Cache directories of Xolotl (${i.servedModelName})";
            serviceConfig = {
              Type = "oneshot";
              RemainAfterExit = true;
              ExecStart = map (
                d: "${pkgs.coreutils}/bin/install -d -m 0700 -o xolotl -g xolotl ${lib.escapeShellArg d}"
              ) (cacheDirs i);
            };
          }
        ) withCache;
      }
    ]
  );
}

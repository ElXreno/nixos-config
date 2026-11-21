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
  cfg = config.${namespace}.services.strata;
  strata = pkgs.${namespace}.strata.override {
    cudaCapabilities = [ config.${namespace}.system.hardware.gpu.nvidia.cudaCapability ];
    ggmlCpuFeatures = cfg.cpuFeatures;
  };
  json = pkgs.formats.json { };
  idle = import ../_idle-backend.nix args;

  flag =
    name: value:
    optionals (value != null) [
      name
      value
    ];

  serveConfig =
    name: i:
    json.generate "strata-${name}.json" (
      lib.recursiveUpdate (
        {
          exe = "${strata}/bin/strata";
          args = [
            "--pack"
            i.pack
            "--native"
            i.native
          ]
          ++ flag "--native-dense-gguf" i.nativeDenseGguf
          ++ flag "--ple-gguf" i.pleGguf
          ++ flag "--embd-gguf" i.embdGguf
          ++ [
            "--expert-profile"
            i.expertProfile
            "--expert-cache"
            "auto"
            "--prefill"
            "auto"
          ]
          ++ optionals (i.mtp != null) [
            "--spec"
            "4"
            "--spec-min-p"
            "0.5"
            "--mtp"
            i.mtp
          ]
          ++ [
            "--max-context"
            (toString i.ctxSize)
            "--kv"
            i.kv
          ]
          ++ optionals (i.kvResident != null) [
            "--kv-resident"
            (toString i.kvResident)
          ]
          ++ optionals (i.conversationCache.mib > 0) [
            "--conversation-cache-mib"
            (toString i.conversationCache.mib)
            "--conversation-cache-slots"
            (toString i.conversationCache.slots)
          ]
          ++ optionals (i.conversationCache.disk.dir != null) (
            [
              "--conversation-cache-spill-dir"
              i.conversationCache.disk.dir
              "--conversation-cache-disk-mib"
              (toString i.conversationCache.disk.mib)
            ]
            ++ optionals i.conversationCache.disk.only [ "--conversation-cache-disk-only" ]
          )
          ++ optionals (i.conversationCache.mib > 0 || i.conversationCache.disk.dir != null) [
            "--conversation-cache-min-tokens"
            (toString i.conversationCache.minTokens)
            "--conversation-cache-n-min"
            (toString i.conversationCache.minTokens)
            "--prompt-cache-root"
            (toString i.conversationCache.minTokens)
          ]
          ++ optionals (i.conversationCache.minFreeMib != null) [
            "--conversation-cache-min-free-mib"
            (toString i.conversationCache.minFreeMib)
          ]
          ++ optionals i.ablation.enable [
            "--control-vector-scaled"
            "${i.ablation.direction}:${toString i.ablation.alpha}"
            "--control-vector-layer-range"
            (toString i.ablation.firstLayer)
            (toString i.ablation.lastLayer)
            "--cvec-mode"
            i.ablation.mode
            "--cvec-dir"
            (if i.ablation.layer == null then "per-layer" else "single:${toString i.ablation.layer}")
          ]
          ++ i.extraArgs;
          inherit (i) parallel;
          cwd = dirOf i.pack;
          tokenizer = "${i.pack}/tokenizer";
          model_name = i.servedModelName;
          log = "${cfg.stateDir}/${name}.log";
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
      name = "strata-${name}-run";
      command = lib.concatStringsSep " " [
        "${strata}/bin/strata-serve --engine strata"
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
        pack = mkOption {
          type = types.str;
          description = "Absolute path to the Strata pack directory (experts.bin, dense.bin, tokenizer).";
        };
        native = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "GGUF with the model's metadata and dense weights (--native); required.";
        };
        nativeDenseGguf = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Dense-only GGUF of a ready-made pack (--native-dense-gguf).";
        };
        pleGguf = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "GGUF holding the per-layer n-gram (PLE) table (--ple-gguf).";
        };
        embdGguf = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "GGUF holding the token embedding (--embd-gguf).";
        };
        mtp = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "MTP draft layer directory; enables speculative decoding.";
        };
        expertProfile = mkOption {
          type = types.str;
          default = "${strata}/share/strata/data/expert-profile.bin";
          defaultText = "The expert profile shipped with the engine.";
          description = "Expert routing profile that seeds the VRAM expert cache.";
        };
        servedModelName = mkOption {
          type = types.str;
          default = "qwen3.8-flash-next";
          description = "Model id advertised on the OpenAI and Anthropic APIs.";
        };
        ctxSize = mkOption {
          type = types.int;
          default = 32768;
          description = "Context window in tokens.";
        };
        kv = mkOption {
          type = types.enum [
            "fp16"
            "int8"
            "k8v4"
            "q4_0"
          ];
          default = "int8";
          description = "KV cache storage type.";
        };
        kvResident = mkOption {
          type = types.nullOr types.int;
          default = null;
          description = "Tokens of KV kept in VRAM while the rest streams from pinned RAM (--kv-resident).";
        };
        conversationCache = {
          mib = mkOption {
            type = types.int;
            default = 0;
            description = "Host RAM budget for parked conversations; 0 disables parking.";
          };
          slots = mkOption {
            type = types.int;
            default = 4;
            description = "Parked conversations kept at most.";
          };
          minTokens = mkOption {
            type = types.int;
            default = 4096;
            description = "Shortest conversation worth parking, shortest disk hit offered and shortest root prefix saved, so short side requests neither evict long conversations nor pin a short shared root.";
          };
          minFreeMib = mkOption {
            type = types.nullOr types.int;
            default = null;
            description = "Available RAM a park must leave behind; null keeps the engine default of 2560.";
          };
          disk = {
            dir = mkOption {
              type = types.nullOr types.str;
              default = "${cfg.conversationCacheDir}/${name}";
              defaultText = "\${conversationCacheDir}/<instance>";
              description = "Directory keeping conversations on disk across switches and restarts; null disables the disk tier.";
            };
            mib = mkOption {
              type = types.int;
              default = 131072;
              description = "Disk budget of the conversation directory.";
            };
            only = mkOption {
              type = types.bool;
              default = true;
              description = "Stream every switched-away conversation to disk and keep none in RAM (--conversation-cache-disk-only).";
            };
          };
        };
        ablation = {
          enable = mkEnableOption "removing a direction from the residual stream";
          direction = mkOption {
            type = types.str;
            default = "${strata}/share/strata/data/experimental-speed-projection/Qwen3.8-Flash-Next-experimental-speed-projection.gguf";
            defaultText = "The refusal-direction control vector shipped with the engine.";
            description = "Control vector GGUF with a `direction.<layer>` tensor per layer.";
          };
          layer = mkOption {
            type = types.nullOr types.int;
            default = null;
            description = "Remove this layer's direction at every layer; null removes each layer's own.";
          };
          alpha = mkOption {
            type = types.float;
            default = 1.0;
            description = "Share of the component removed at each cut.";
          };
          mode = mkOption {
            type = types.enum [
              "project"
              "delta"
            ];
            default = "project";
            description = "Cut the direction from the accumulated residual after each layer, or from each attention and FFN output before it is written.";
          };
          firstLayer = mkOption {
            type = types.int;
            default = 4;
            description = "First layer whose output is cut.";
          };
          lastLayer = mkOption {
            type = types.int;
            default = 44;
            description = "Last layer whose output is cut.";
          };
        };
        parallel = mkOption {
          type = types.int;
          default = 1;
          description = "Requests decoded together in batch slots; 1 serves one at a time.";
        };
        settings = mkOption {
          inherit (json) type;
          default = { };
          description = "Keys merged over the generated strata-serve config, e.g. sampling or reasoning_effort.";
        };
        environment = mkOption {
          type = types.attrsOf types.str;
          default = { };
          description = "Engine tuning variables (STRATA_*).";
        };
        memoryMax = mkOption {
          type = types.str;
          default = "80G";
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
  options.${namespace}.services.strata = {
    enable = mkEnableOption "running Strata inference servers";

    stateDir = mkOption {
      type = types.str;
      default = "/var/lib/strata";
      description = "State directory holding the engine logs.";
    };

    conversationCacheDir = mkOption {
      type = types.str;
      default = "${cfg.stateDir}/conversations";
      defaultText = "\${stateDir}/conversations";
      description = "Parent of the per-instance disk conversation caches.";
    };

    cpuFeatures = mkOption {
      type = types.listOf types.str;
      default = pkgs.${namespace}.strata.ggmlCpuFeatures;
      description = "GGML_<feature> CPU flags the CPU expert kernels are built with.";
    };

    instances = mkOption {
      type = types.attrsOf instanceOpts;
      default = { };
    };
  };

  config = mkIf cfg.enable (
    lib.mkMerge [
      (idle.units {
        name = "strata";
        label = "Strata";
        inherit (cfg) stateDir instances;
        describe = i: i.servedModelName;
        backend = name: i: {
          requires = lib.optional (i.conversationCache.disk.dir != null) "strata-${name}-cache-dir.service";
          after = lib.optional (i.conversationCache.disk.dir != null) "strata-${name}-cache-dir.service";
          path = [ pkgs.bash ];
          environment = {
            HOME = cfg.stateDir;
          }
          // i.environment;
          serviceConfig = {
            ExecStart = runner name i;
            ReadWritePaths = [
              cfg.stateDir
            ]
            ++ lib.optional (i.conversationCache.disk.dir != null) i.conversationCache.disk.dir;
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
        assertions = lib.concatLists (
          lib.mapAttrsToList (name: i: [
            {
              assertion = i.conversationCache.disk.only -> i.conversationCache.disk.dir != null;
              message = "${namespace}.services.strata.instances.${name}: conversationCache.disk.only needs conversationCache.disk.dir";
            }
            {
              assertion = i.native != null;
              message = "${namespace}.services.strata.instances.${name}: native must be set";
            }
          ]) cfg.instances
        );

        systemd.services = lib.mapAttrs' (
          name: i:
          lib.nameValuePair "strata-${name}-cache-dir" {
            description = "Conversation cache directory of Strata (${i.servedModelName})";
            serviceConfig = {
              Type = "oneshot";
              RemainAfterExit = true;
              ExecStart = "${pkgs.coreutils}/bin/install -d -m 0750 -o strata -g strata ${lib.escapeShellArg i.conversationCache.disk.dir}";
            };
          }
        ) (lib.filterAttrs (_: i: i.conversationCache.disk.dir != null) cfg.instances);
      }
    ]
  );
}

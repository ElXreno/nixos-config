{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    concatStringsSep
    escapeShellArg
    escapeShellArgs
    getExe
    mkEnableOption
    mkIf
    mkMerge
    mkOption
    optionalAttrs
    optionalString
    optionals
    types
    ;
  cfg = config.${namespace}.programs.memory;
  cc = config.programs.claude-code;
  home = config.home.homeDirectory;
  embedOn = cfg.embed.url != null;

  wrapper = pkgs.symlinkJoin {
    name = "mem";
    paths = [ cfg.package ];
    nativeBuildInputs = [ pkgs.makeWrapper ];
    postBuild = ''
      wrapProgram $out/bin/mem \
        --set-default MEM_PROTECTED_DIRS ${escapeShellArg (concatStringsSep ":" cfg.protectedDirs)}${optionalString embedOn " \\\n  --set-default MEM_EMBED_URL ${escapeShellArg cfg.embed.url} \\\n  --set-default MEM_EMBED_MODEL ${escapeShellArg cfg.embed.model} \\\n  --set-default MEM_EMBED_QUERY_PREFIX ${escapeShellArg cfg.embed.queryPrefix} \\\n  --set-default MEM_EMBED_DOC_PREFIX ${escapeShellArg cfg.embed.docPrefix}"}${
          optionalString (
            cfg.rerank.url != null
          ) " \\\n  --set-default MEM_RERANK_URL ${escapeShellArg cfg.rerank.url}"
        }
    '';
    meta.mainProgram = "mem";
  };

  hook = event: args: timeout: {
    type = "command";
    command = "${getExe wrapper} hook ${event}${
      optionalString (args != [ ]) " ${escapeShellArgs args}"
    }";
    inherit timeout;
  };

  embedArgs =
    {
      timeout,
      budget ? [ ],
      shadow ? [ ],
    }:
    optionals embedOn (
      [
        "--embed-timeout"
        timeout
      ]
      ++ budget
      ++ optionals (cfg.recall.pool != "lexical") [
        "--vec-pool"
        cfg.recall.pool
      ]
      ++ shadow
      ++ cfg.recall.extraArgs
    );

  promptArgs = embedArgs {
    timeout = "5s";
    budget = [
      "--budget"
      "9s"
    ];
    shadow = optionals (cfg.shadowPool != null) [
      "--shadow-vec-pool"
      cfg.shadowPool
    ];
  };

  agentArgs = embedArgs { timeout = "2s"; } ++ [
    "--budget"
    (if embedOn then "9s" else "3s")
    "--agent-recall"
  ];

  failureArgs = embedArgs {
    timeout = "2s";
    budget = [
      "--budget"
      "9s"
    ];
  };

  memorySection = ''
    ## Memory
    - Memory lives in `mem`: core records and a topic map load at session start, relevant records arrive with each prompt
    - Before acting on a topic look it up with `mem search QUERY` or `mem get KEY...` (`--body` prints only the text, several keys in one call); write only through `mem put` (`--update` for the same topic, `--supersedes` when a fact changed, `--ttl 30d` or `--verify` for state)
    - `mem put` takes the key first and the body in `--body` or on stdin: new record `mem put KEY --kind rule|fact|state|incident|howto|log --scope S --title T --body TEXT` (or `<<'E'` instead of `--body`), same topic `mem put KEY --update --body TEXT` (kind, scopes and title are kept)
    - Change part of a record with `mem edit KEY --old TEXT --new TEXT` (one exact occurrence, like the Edit tool) instead of retyping the whole body through `--update`
    - A pointer or a search hit marked `EXPIRED` is a state record nobody verified in time: check the fact, then `mem verify KEY` or `mem retire KEY --why TEXT`
    - `mem stats` reports how recall behaved: injections, embedder outcomes, trigger fires, expiring state
    - The old files under `~/.claude/projects/*/memory` are read-only history: never edit them
    - Save durable facts, preferences and corrections proactively, as before
    - Record bodies are plain English; my own words stay verbatim, in the language I wrote them, inside «»
    - One record per topic: update or supersede it instead of appending history, and keep only the current state in the body
    - Scope every record to where it holds: `global` for my preferences and rules that apply in every project, `project:<repo>` for one repository, `host:<name>` for one machine; combine them when both apply
  '';

  coreImport = "@~/.claude/memory-core.md";
in
{
  options.${namespace}.programs.memory = {
    enable = mkEnableOption "the agent memory store";

    package = mkOption {
      type = types.package;
      default = pkgs.${namespace}.mem;
      description = "mem package.";
    };

    protectedDirs = mkOption {
      type = types.listOf (types.strMatching "/.*");
      default = [ "${home}/.claude/projects/*/memory" ];
      description = "Directories besides the database directory that Write, Edit, MultiEdit and NotebookEdit may not touch; `*` matches one path component.";
    };

    disableBuiltin = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to turn off the built-in auto memory of Claude Code.";
    };

    embed = {
      url = mkOption {
        type = types.nullOr types.str;
        default = null;
        example = "http://127.0.0.1:8096";
        description = "OpenAI-compatible embeddings endpoint; null keeps mem lexical only.";
      };

      model = mkOption {
        type = types.str;
        default = "jinaai/jina-embeddings-v5-text-small-retrieval";
        description = "Embedding model id stored with each vector.";
      };

      queryPrefix = mkOption {
        type = types.str;
        default = "Query: ";
        description = "Prefix the model expects on queries.";
      };

      docPrefix = mkOption {
        type = types.str;
        default = "Document: ";
        description = "Prefix the model expects on documents.";
      };

      reindexInterval = mkOption {
        type = types.str;
        default = "5min";
        description = "How often new and changed records get embedded.";
      };
    };

    rerank.url = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "http://127.0.0.1:8101";
      description = "llama.cpp /v1/rerank endpoint that rescores the vector candidates; null keeps the cosine gate.";
    };

    subagents = {
      enable = mkOption {
        type = types.bool;
        default = true;
        description = "Whether subagents get the recall of their launch prompt and a topic map: PreToolUse on Agent and Task queues the recall, SubagentStart hands it over, and an agent spawned by the Workflow tool gets it at its first tool call.";
      };

      extraArgs = mkOption {
        type = types.listOf types.str;
        default = [ ];
        example = [
          "--map-chars"
          "1000"
          "--pending-ttl"
          "90s"
        ];
        description = "Extra flags for `mem hook subagent-start`.";
      };
    };

    shadowPool = mkOption {
      type = types.nullOr (
        types.enum [
          "reg"
          "fused-reg"
          "all"
          "fused-all"
        ]
      );
      default = null;
      description = "Vector pool decided next to the live one and logged to shadow.jsonl, never injected.";
    };

    recall = {
      pool = mkOption {
        type = types.enum [
          "lexical"
          "reg"
          "fused-reg"
          "all"
          "fused-all"
        ];
        default = "lexical";
        description = "Pool the prompt hook injects from; vector pools need embed.url.";
      };

      extraArgs = mkOption {
        type = types.listOf types.str;
        default = [ ];
        example = [
          "--reg-cos"
          "0.65"
          "--reg-margin"
          "0.05"
        ];
        description = "Extra flags for `mem hook user-prompt-submit`, such as gate thresholds. They are also passed to the Agent recall of `mem hook pre-tool-use` and to `mem hook post-tool-use-failure`, so only flags that all three events accept with the same meaning belong here: the gate and vector flags, not `--short`, `--tail` or `--skip-prefix`, and not `--max-chars`, which is the trigger budget on pre-tool-use.";
      };

      onToolFailure = {
        enable = mkEnableOption "pointers to related records after a failed tool call";

        matcher = mkOption {
          type = types.str;
          default = "Bash";
          description = "Tools whose failures get the recall, a PostToolUseFailure matcher.";
        };
      };
    };
  };

  config = mkIf cfg.enable (mkMerge [
    {
      home.packages = [ wrapper ];

      systemd.user = {
        services.mem-expire = {
          Unit.Description = "Mark mem state records past their ttl as expired";
          Service = {
            Type = "oneshot";
            ExecStart = "${getExe wrapper} expire";
          };
        };
        timers.mem-expire = {
          Unit.Description = "Daily mem expire";
          Timer = {
            OnCalendar = "daily";
            Persistent = true;
          };
          Install.WantedBy = [ "timers.target" ];
        };
      };
    }

    (mkIf embedOn {
      systemd.user = {
        services.mem-reindex = {
          Unit.Description = "Embed new and changed mem records";
          Service = {
            Type = "oneshot";
            ExecStart = "${getExe wrapper} reindex";
          };
        };
        timers.mem-reindex = {
          Unit.Description = "Periodic mem reindex";
          Timer = {
            OnStartupSec = "2min";
            OnUnitActiveSec = cfg.embed.reindexInterval;
          };
          Install.WantedBy = [ "timers.target" ];
        };
      };
    })

    (mkIf cc.enable {
      ${namespace}.programs.claude-code.memoryRules = memorySection + "\n" + coreImport + "\n";

      programs.claude-code.settings = {
        autoMemoryEnabled = mkIf cfg.disableBuiltin false;

        hooks = {
          SessionStart = [
            {
              matcher = "startup|resume|clear|compact";
              hooks = [ (hook "session-start" [ "--no-usage" ] 5) ];
            }
          ];

          UserPromptSubmit = [
            { hooks = [ (hook "user-prompt-submit" promptArgs (if embedOn then 12 else 5)) ]; }
          ];

          PreToolUse = [
            {
              matcher = "*";
              hooks = [
                (hook "pre-tool-use" (optionals cfg.subagents.enable agentArgs) (
                  if embedOn && cfg.subagents.enable then 12 else 5
                ))
              ];
            }
          ];
        }
        // optionalAttrs cfg.subagents.enable {
          SubagentStart = [
            { hooks = [ (hook "subagent-start" cfg.subagents.extraArgs 5) ]; }
          ];
        }
        // optionalAttrs cfg.recall.onToolFailure.enable {
          PostToolUseFailure = [
            {
              inherit (cfg.recall.onToolFailure) matcher;
              hooks = [ (hook "post-tool-use-failure" failureArgs (if embedOn then 12 else 5)) ];
            }
          ];
        };
      };
    })
  ]);
}

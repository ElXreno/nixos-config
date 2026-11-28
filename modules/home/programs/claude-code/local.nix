{
  config,
  osConfig,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.claude-code;
  gateway = osConfig.${namespace}.services.llm-gateway;

  models = lib.listToAttrs (map (m: lib.nameValuePair m.peer m) gateway.models);

  settings = pkgs.writeText "claude-local-settings.json" (
    builtins.toJSON {
      modelPicker = {
        replaceBuiltInOptions = true;
        options = map (m: {
          inherit (m) model;
          description = "${m.peer}, ${toString (m.ctxSize / 1024)}k context";
        }) gateway.models;
      };
      env.OBSERVER_EVERY_STEPS = toString cfg.local.observerEverySteps;
    }
  );

  launch = ''
    export ANTHROPIC_BASE_URL=http://127.0.0.1:${toString gateway.port}
    export ANTHROPIC_API_KEY=
    export ANTHROPIC_AUTH_TOKEN=local
    export ANTHROPIC_MODEL="$model"
    export ANTHROPIC_DEFAULT_OPUS_MODEL="$model"
    export ANTHROPIC_DEFAULT_SONNET_MODEL="$model"
    export ANTHROPIC_DEFAULT_FABLE_MODEL="$model"
    export CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1
    export CLAUDE_CODE_NO_MODEL_FALLBACK=1
    export CLAUDE_CODE_MAX_CONTEXT_TOKENS="$ctx"
    export CLAUDE_CODE_MAX_OUTPUT_TOKENS=${toString cfg.local.maxOutputTokens}
    export CLAUDE_CODE_ATTRIBUTION_HEADER=0
    export API_TIMEOUT_MS=3600000
    export CLAUDE_STREAM_IDLE_TIMEOUT_MS=3600000
    export CLAUDE_CODE_GOAL_CHECK_TIMEOUT_MS=${toString (gateway.responseHeaderTimeout * 1000)}
    export BASH_MAX_TIMEOUT_MS=10800000
    export ENABLE_TOOL_SEARCH="$toolsearch"
    export CLAUDE_LOCAL_COMMAND="claude-local-$peer"
    export MEM_AGENT=${cfg.local.memAgent}
    exec claude --model "$model" --effort ${cfg.local.effort} --settings ${settings} --permission-mode ${cfg.local.permissionMode} "$@"
  '';

  wrapper =
    name: m:
    pkgs.writeShellApplication {
      inherit name;
      runtimeInputs = [ config.programs.claude-code.package ];
      text = ''
        peer=${m.peer}
        model=${m.model}
        ctx=${toString m.ctxSize}
        toolsearch=${lib.boolToString m.toolSearch}
      ''
      + launch;
    };

  ordered =
    let
      first = models.${cfg.local.default};
    in
    [ first ] ++ lib.filter (m: m.peer != first.peer) gateway.models;

  autoWrapper = pkgs.writeShellApplication {
    name = "claude-local";
    runtimeInputs = [
      config.programs.claude-code.package
      osConfig.systemd.package
    ];
    text = ''
      peer=${(lib.head ordered).peer}
      model=${(lib.head ordered).model}
      ctx=${toString (lib.head ordered).ctxSize}
      toolsearch=${lib.boolToString (lib.head ordered).toolSearch}
      for entry in ${
        lib.escapeShellArgs (
          map (
            m: "${m.unit}|${m.peer}|${m.model}|${toString m.ctxSize}|${lib.boolToString m.toolSearch}"
          ) ordered
        )
      }; do
        IFS='|' read -r unit name candidate size search <<< "$entry"
        state=$(systemctl is-active "$unit" || true)
        if [[ $state == active || $state == activating || $state == reloading ]]; then
          peer=$name
          model=$candidate
          ctx=$size
          toolsearch=$search
          break
        fi
      done
      echo "claude-local: $model" >&2
    ''
    + launch;
  };
in
{
  options.${namespace}.programs.claude-code.local = {
    enable = mkEnableOption "adding claude-local, Claude Code pointed at the host's llm-gateway" // {
      default = gateway.enable;
      defaultText = lib.literalExpression "osConfig.\${namespace}.services.llm-gateway.enable";
    };
    default = mkOption {
      type = types.str;
      default =
        if models != { } then
          (lib.head gateway.models).peer
        else
          throw "claude-local: the host's llm-gateway routes no model";
      description = "Gateway peer claude-local prefers; it falls back to the first running backend, and to this one when none runs.";
    };
    maxOutputTokens = mkOption {
      type = types.int;
      default = 32768;
      description = "Output token cap Claude Code requests.";
    };
    observerEverySteps = mkOption {
      type = types.ints.unsigned;
      default = 20;
      description = "Tool calls between observer passes in claude-local; 0 leaves only the pass before a compaction.";
    };
    memAgent = mkOption {
      type = types.strMatching "[a-z0-9][a-z0-9_.+@-]*";
      default = "local";
      description = "Name the local models write memory under: mem keeps their records out of the sessions of the cloud models, and they read every record.";
    };
    effort = mkOption {
      type = types.enum [
        "low"
        "medium"
        "high"
        "xhigh"
        "max"
      ];
      default = "max";
      description = "Effort level claude-local starts with; /effort or a later --effort still changes it.";
    };
    permissionMode = mkOption {
      type = types.enum [
        "default"
        "acceptEdits"
        "plan"
        "dontAsk"
        "bypassPermissions"
        "auto"
      ];
      default = "bypassPermissions";
      description = "Permission mode for claude-local; auto needs the Anthropic classifier, so the default skips permission prompts and leaves the guarding to hooks.";
    };
  };

  config = mkIf (cfg.enable && cfg.local.enable) {
    assertions = [
      {
        assertion = gateway.enable;
        message = "${namespace}.programs.claude-code.local needs ${namespace}.services.llm-gateway on the host";
      }
    ];

    home.packages = [ autoWrapper ] ++ lib.mapAttrsToList (name: wrapper "claude-local-${name}") models;
  };
}

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
    optionalAttrs
    types
    ;
  cfg = config.${namespace}.programs.claude-code;

  script =
    name: replacements:
    pkgs.replaceVarsWith {
      name = "claude-${name}";
      src = ./${name}.sh;
      inherit replacements;
      isExecutable = true;
      buildInputs = [ pkgs.runtimeShellPackage ];
    };

  mkPyTool =
    name: script:
    pkgs.writeShellScriptBin name ''
      exec ${lib.getExe pkgs.python3} ${script} "$@"
    '';

  sweepLedger = mkPyTool "sweep-ledger" ./skills/exhaustive-sweep/ledger.py;
  memoryReconcile = mkPyTool "memory-reconcile" ./skills/memory-migrate/reconcile.py;

  officialPlugins = pkgs.fetchFromGitHub {
    owner = "anthropics";
    repo = "claude-plugins-official";
    rev = "fa4eb54397ee2faccb6940512a19c017cb1337f3";
    hash = "sha256-FqB+THQsn//s7PIFITa/k9RWOsYytrH50scCuuLgsYw=";
  };
in
{
  options.${namespace}.programs.claude-code = {
    enable = mkEnableOption "managing Claude Code";

    statusLineHooks = mkOption {
      type = types.listOf types.path;
      default = [ ];
      description = "Commands run with the status line JSON on stdin.";
    };

    observer = {
      enable =
        mkEnableOption "the observer mod, a registry of examined objects and closed approaches kept outside the conversation"
        // {
          default = true;
        };
      everySteps = mkOption {
        type = types.ints.unsigned;
        default = 0;
        description = "Tool calls between observer passes; 0 leaves only the pass before a compaction.";
      };
    };

    memoryRules = mkOption {
      type = types.lines;
      default = builtins.readFile ./memory-builtin.md;
      description = "The `## Memory` section appended to the global CLAUDE.md.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.programs = {
      agent-browser.enable = true;
      camoufox-cli.enable = true;
      mcp.enable = true;
    };

    home.packages = [
      sweepLedger
      memoryReconcile
      pkgs.${namespace}.rea-full
    ];

    home.file.".claude/workflows" = {
      source = ./workflows;
      recursive = true;
    };

    # workaround for https://github.com/nix-community/home-manager/issues/9906
    home.file."${config.programs.claude-code.configDir}/skills/goal".source = ./plugins/goal;
    home.file."${config.programs.claude-code.configDir}/skills/observer" = mkIf cfg.observer.enable {
      source = ./plugins/observer;
    };

    # /effort, /model and /tui rewrite settings.json, which fails on a store symlink.
    home.file."${config.programs.claude-code.configDir}/settings.json".enable = false;

    home.file."${config.programs.claude-code.configDir}/CLAUDE.md".source = pkgs.writeText "CLAUDE.md" (
      lib.removeSuffix "\n" (builtins.readFile ./CLAUDE.md) + "\n\n" + cfg.memoryRules
    );

    home.activation.claudeCodeSettings = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
      run ${pkgs.coreutils}/bin/install -Dm644 \
        ${config.home.file."${config.programs.claude-code.configDir}/settings.json".source} \
        "${config.programs.claude-code.configDir}/settings.json"
    '';

    programs.claude-code = {
      enable = true;
      enableMcpIntegration = true;

      commandsDir = ./commands;

      skills.agent-browser = "${pkgs.agent-browser}/share/agent-browser/skills/agent-browser";
      skills.camoufox = "${pkgs.${namespace}.camoufox-cli}/share/camoufox-cli/skills/camoufox";
      skills.memory-migrate = ./skills/memory-migrate;
      skills.exhaustive-sweep = ./skills/exhaustive-sweep;
      skills.digger = mkIf osConfig.${namespace}.services.digger.enable ./skills/digger;
      skills.reverse-engineer-anything = pkgs.${namespace}.rea-full.skill;

      plugins.claude-security = "${officialPlugins}/plugins/claude-security";
      plugins.frontend-design = "${officialPlugins}/plugins/frontend-design";

      outputStyles.terse-autonomous = ./output-styles/terse-autonomous.md;

      agents = {
        researcher = ./agents/researcher.md;
        verifier = ./agents/verifier.md;
        source-spelunker = ./agents/source-spelunker.md;
      };

      settings = {
        model = "claude-opus-5-5";
        fallbackModel = [ "claude-opus-5" ];
        modelSettings = lib.genAttrs [ "claude-opus-5-5" "claude-opus-5" "claude-fable-5-1" ] (_: {
          effortLevel = "high";
        });
        outputStyle = "Terse autonomous";
        skipDangerousModePermissionPrompt = true;
        crossSessionInbound = "accept";
        worktree.baseRef = "head";
        verbose = true;
        awaySummaryEnabled = false;
        idleCompaction = false;
        cleanupPeriodDays = 36500;
        preferredNotifChannel = "notifications_disabled";
        leftArrowOpensAgents = false;
        tui = "fullscreen";
        attribution = {
          commit = "";
          pr = "";
          sessionUrl = false;
        };

        statusLine = {
          type = "command";
          command = "${script "statusline" {
            jq = lib.getExe pkgs.jq;
            git = "${pkgs.git}/bin/git";
            hooks = lib.escapeShellArgs cfg.statusLineHooks;
          }}";
          refreshInterval = 30;
        };

        subagentStatusLine = {
          type = "command";
          command = "${script "subagent-statusline" { jq = lib.getExe pkgs.jq; }}";
        };

        hooks.PostToolUse = [
          {
            matcher = "Edit|MultiEdit|Write";
            hooks = [
              {
                type = "command";
                command = "${script "fast-lint" {
                  jq = lib.getExe pkgs.jq;
                  ruff = lib.getExe pkgs.ruff;
                  hadolint = lib.getExe pkgs.hadolint;
                  yamllint = lib.getExe pkgs.yamllint;
                  golangci = lib.getExe pkgs.golangci-lint;
                  timeout = "${pkgs.coreutils}/bin/timeout";
                }}";
                timeout = 15;
              }
            ];
          }
        ];

        env = {
          CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL = "1";
          CLAUDE_CODE_RESUME_INTERRUPTED_TURN = "1";
          CLAUDE_CODE_MAX_WEB_SEARCHES_PER_SESSION = "2000";
          CLAUDE_CODE_ENABLE_TODO_TOOLS = "1";
          # opts out of the tengu_thrifty_sonic cohort
          CLAUDE_CODE_THRIFTY_SONIC = "0";
          # overrides the fullscreen boot-canary auto-disable
          CLAUDE_CODE_NO_FLICKER = "1";
          # Background waiters must outlive a PSI spike caused by a VM test or the ARC.
          CLAUDE_CODE_DISABLE_BG_SHELL_PRESSURE_REAP = "1";
        }
        // optionalAttrs (cfg.observer.everySteps != 0) {
          OBSERVER_EVERY_STEPS = toString cfg.observer.everySteps;
        };
      };
    };
  };
}

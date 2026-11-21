{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf optionalAttrs;
  cfg = config.${namespace}.programs.claude-code;
  home = config.home.homeDirectory;
  guard = config.${namespace}.programs.agent-guard;
  cc = guard.harnesses.claude-code;
  editTools = "Edit|MultiEdit|Write";
  hook = {
    type = "command";
    command = "${cc.command} hook";
  };
in
{
  config = mkIf cfg.enable {
    ${namespace}.programs.agent-guard = {
      defaultHarness = "claude-code";
      harnesses.claude-code = {
        transcripts = "${home}/.claude/projects";
        shellReadRoots = [ "${home}/.claude" ];
        createRoots = [ "${home}/.claude" ];
        memoryPaths = [
          "${home}/.claude/projects/*/memory/"
          "${home}/.claude/agent-memory/"
          ".claude/agent-memory/"
          ".claude/agent-memory-local/"
        ];
      };
    };

    programs.claude-code = {
      agents.log-forensics = pkgs.replaceVars ./agents/log-forensics.md {
        matcher = editTools;
        memguard = "${cc.command} confine log-forensics";
      };

      settings.hooks = {
        PreToolUse = [
          {
            matcher = "Bash|${editTools}";
            hooks = [ hook ];
          }
        ];
        PermissionRequest = [ { hooks = [ hook ]; } ];
      }
      // optionalAttrs guard.power.enable {
        PostToolUse = [
          {
            matcher = "*";
            hooks = [ hook ];
          }
        ];
        UserPromptSubmit = [ { hooks = [ hook ]; } ];
      };

      settings.env.AGENT_GUARD_ASK_TIMEOUT_MS = toString (guard.askTimeoutMinutes * 60 * 1000);
    };
  };
}

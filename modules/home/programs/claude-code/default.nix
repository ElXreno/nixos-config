{
  config,
  namespace,
  inputs,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.claude-code;
in
{
  options.${namespace}.programs.claude-code = {
    enable = mkEnableOption "Whether to manage Claude Code.";
  };

  config = mkIf cfg.enable {
    home.packages = [ pkgs.rtk ];

    programs.claude-code = {
      enable = true;
      enableMcpIntegration = true;

      context = ./CLAUDE.md;
      commandsDir = ./commands;

      plugins = [ inputs.caveman ];

      lspServers = {
        bash = {
          command = lib.getExe pkgs.bash-language-server;
          args = [ "start" ];
          extensionToLanguage = {
            ".sh" = "shellscript";
            ".bash" = "shellscript";
          };
        };
        go = {
          command = lib.getExe pkgs.gopls;
          extensionToLanguage = {
            ".go" = "go";
          };
        };
        json = {
          command = lib.getExe pkgs.vscode-json-languageserver;
          args = [ "--stdio" ];
          extensionToLanguage = {
            ".json" = "json";
          };
        };
        nix = {
          command = lib.getExe pkgs.nixd;
          extensionToLanguage = {
            ".nix" = "nix";
          };
        };
        opentofu = {
          command = lib.getExe pkgs.tofu-ls;
          args = [ "serve" ];
          extensionToLanguage = {
            ".tf" = "terraform";
            ".tfvars" = "terraform-vars";
          };
        };
        python = {
          command = lib.getExe' pkgs.basedpyright "basedpyright-langserver";
          args = [ "--stdio" ];
          extensionToLanguage = {
            ".py" = "python";
            ".pyi" = "python";
          };
        };
        rust = {
          command = lib.getExe pkgs.rust-analyzer;
          extensionToLanguage = {
            ".rs" = "rust";
          };
        };
        toml = {
          command = lib.getExe pkgs.taplo;
          args = [
            "lsp"
            "stdio"
          ];
          extensionToLanguage = {
            ".toml" = "toml";
          };
        };
        typescript = {
          command = lib.getExe pkgs.typescript-language-server;
          args = [ "--stdio" ];
          extensionToLanguage = {
            ".ts" = "typescript";
            ".tsx" = "typescriptreact";
            ".js" = "javascript";
            ".jsx" = "javascriptreact";
          };
        };
        yaml = {
          command = lib.getExe pkgs.yaml-language-server;
          args = [ "--stdio" ];
          extensionToLanguage = {
            ".yaml" = "yaml";
            ".yml" = "yaml";
          };
        };
      };

      settings = {
        model = "claude-fable-5";
        verbose = true;
        includeCoAuthoredBy = false;
        gitAttribution = false;

        statusLine = {
          type = "command";
          command = "${pkgs.writeShellScript "claude-statusline" (
            builtins.replaceStrings
              [ "@jq@" "@git@" ]
              [
                (lib.getExe pkgs.jq)
                "${pkgs.git}/bin/git"
              ]
              (builtins.readFile ./statusline.sh)
          )}";
          refreshInterval = 30;
        };

        hooks = {
          PreToolUse = [
            {
              matcher = "Bash";
              hooks = [
                {
                  type = "command";
                  command = "${pkgs.writeShellScript "rtk-rewrite" ''
                    set -uo pipefail

                    input=$(cat)
                    cmd=$(${lib.getExe pkgs.jq} -r '.tool_input.command // empty' <<<"$input")
                    [ -z "$cmd" ] && exit 0

                    rewritten=$(${lib.getExe pkgs.rtk} rewrite "$cmd" 2>/dev/null)
                    rc=$?

                    case $rc in
                      0) [ "$cmd" = "$rewritten" ] && exit 0 ;;
                      3) ;;
                      *) exit 0 ;;
                    esac

                    if [ "$rc" -eq 3 ]; then
                      ${lib.getExe pkgs.jq} -c --arg cmd "$rewritten" \
                        '.tool_input.command = $cmd | {hookSpecificOutput: {hookEventName: "PreToolUse", updatedInput: .tool_input}}' <<<"$input"
                    else
                      ${lib.getExe pkgs.jq} -c --arg cmd "$rewritten" \
                        '.tool_input.command = $cmd | {hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "allow", permissionDecisionReason: "RTK auto-rewrite", updatedInput: .tool_input}}' <<<"$input"
                    fi
                  ''}";
                }
              ];
            }
          ];

          SubagentStart = [
            {
              hooks = [
                {
                  type = "command";
                  command = "${pkgs.writeShellScript "caveman-subagent-inject" ''
                    set -eu

                    flag="''${CLAUDE_CONFIG_DIR:-$HOME/.claude}/.caveman-active"
                    [ -f "$flag" ] || exit 0

                    mode=$(${pkgs.coreutils}/bin/tr -d '[:space:]' <"$flag" 2>/dev/null || true)
                    case "$mode" in
                      lite | full | ultra | wenyan-lite | wenyan | wenyan-full | wenyan-ultra) ;;
                      *) exit 0 ;;
                    esac

                    body=$(${pkgs.gnused}/bin/sed '1,/^---$/d' "${inputs.caveman}/skills/caveman/SKILL.md")

                    rules="CAVEMAN MODE ACTIVE — level: $mode

                    $body

                    ## Subagent context

                    Your final returned message IS the deliverable — apply caveman to it. Structured or schema output the parent parses: write fields normal."

                    printf '%s' "$rules" | ${lib.getExe pkgs.jq} -Rsc \
                      '{hookSpecificOutput: {hookEventName: "SubagentStart", additionalContext: .}}'
                  ''}";
                }
              ];
            }
          ];

          PostToolUse = [
            {
              matcher = "Edit|MultiEdit|Write";
              hooks = [
                {
                  type = "command";
                  command = "${pkgs.writeShellScript "claude-pre-commit" ''
                    set -uo pipefail

                    file=$(${lib.getExe pkgs.jq} -r '.tool_input.file_path')
                    [ -f "$file" ] || exit 0

                    repo=$(${pkgs.git}/bin/git -C "$(dirname "$file")" rev-parse --show-toplevel 2>/dev/null) || exit 0

                    if [ ! -f "$repo/.pre-commit-config.yaml" ] && [ ! -f "$repo/.pre-commit-config.yml" ]; then
                      exit 0
                    fi

                    if ! command -v pre-commit >/dev/null 2>&1; then
                      echo "pre-commit config found at $repo but pre-commit not in PATH" >&2
                      exit 1
                    fi

                    output=$(cd "$repo" && pre-commit run --files "$file" 2>&1)
                    rc=$?
                    if [ $rc -ne 0 ]; then
                      echo "$output" >&2
                      exit 2
                    fi
                  ''}";
                }
              ];
            }
          ];
        };

        env = {
          CAVEMAN_DEFAULT_MODE = "ultra";
          CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL = "1";
          CLAUDE_CODE_RESUME_INTERRUPTED_TURN = "1";
          # Breaks stuff: bwrap: Can't create file at /home/elxreno/.bash_profile: No such file or directory
          # CLAUDE_CODE_SUBPROCESS_ENV_SCRUB = "1";
        };

        permissions.allow = [
          "Bash(chmod:*)"
          "Bash(curl:*)"
          "Bash(echo:*)"
          "Bash(fd:*)"
          "Bash(find:*)"
          "Bash(gh api:*)"
          "Bash(gh issue:*)"
          "Bash(gh pr:*)"
          "Bash(gh release:*)"
          "Bash(gh repo:*)"
          "Bash(gh run:*)"
          "Bash(gh search:*)"
          "Bash(git fetch:*)"
          "Bash(git rebase:*)"
          "Bash(git pull:*)"
          "Bash(git reflog:*)"
          "Bash(git cherry-pick:*)"
          "Bash(git cp:*)"
          "Bash(glab ci:*)"
          "Bash(grep:*)"
          "Bash(helm dependency build:*)"
          "Bash(helm dependency update:*)"
          "Bash(helm show:*)"
          "Bash(helm template:*)"
          "Bash(jq)"
          "Bash(kubectl describe:*)"
          "Bash(kubectl get:*)"
          "Bash(kubectl logs:*)"
          "Bash(kubectl version:*)"
          "Bash(kubectl wait:*)"
          "Bash(ls:*)"
          "Bash(nix:*)"
          "Bash(nix-shell:*)"
          "Bash(python3:*)"
          "Bash(test:*)"
          "Bash(wait)"
          "Bash(wc:*)"
          "Bash(xargs:*)"
          "Read(//home/*/.claude/**)"
          "Read(//tmp/**)"
          "WebSearch"
          "mcp__plugin_claude-code-home-manager_ghidra-mcp__*"
          "mcp__plugin_claude-code-home-manager_nixos__*"
          "mcp__plugin_claude-code-home-manager_playwright__*"
        ];
      };
    };
  };
}

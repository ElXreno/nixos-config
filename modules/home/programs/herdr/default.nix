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
    mkDefault
    mkIf
    mkEnableOption
    mkMerge
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.herdr;

  assets = "${pkgs.herdr.src}/src/integration/assets";

  quietSounds =
    pkgs.runCommand "herdr-quiet-sounds" { nativeBuildInputs = [ pkgs.ffmpeg-headless ]; }
      ''
        mkdir $out
        for sound in done request; do
          ffmpeg -nostdin -loglevel error -i ${pkgs.herdr.src}/assets/sounds/$sound.mp3 \
            -filter:a volume=-18dB -codec:a libmp3lame -b:a 256k $out/$sound.mp3
        done
      '';

  claudeHook = pkgs.runCommand "herdr-claude-agent-state" { } ''
    install -Dm755 ${assets}/claude/herdr-agent-state.sh $out
    substituteInPlace $out \
      --replace-fail 'command -v python3 >/dev/null 2>&1 || exit 0' ':' \
      --replace-fail "python3 - <<'PY'" "${pkgs.python3Minimal}/bin/python3 - <<'PY'"
  '';

  claudeResumeReport = pkgs.writeShellApplication {
    name = "herdr-claude-resume-report";
    runtimeInputs = [
      pkgs.herdr
      pkgs.coreutils
    ];
    text = ''
      session=$1
      for delay in 2 3 5 10 20; do
        sleep "$delay"
        if herdr pane report-agent-session "$HERDR_PANE_ID" --source herdr:claude --agent claude \
          --seq "$(date +%s%N)" --agent-session-id "$session" \
          -- "$CLAUDE_LOCAL_COMMAND" --resume "$session" >/dev/null 2>&1; then
          exit 0
        fi
      done
    '';
  };

  claudeResumeHook = pkgs.writeShellApplication {
    name = "herdr-claude-resume-hook";
    runtimeInputs = [
      pkgs.jq
      pkgs.util-linux
    ];
    text = ''
      input=$(cat)
      [[ ''${HERDR_ENV:-} == 1 && -n ''${HERDR_PANE_ID:-} && -n ''${CLAUDE_LOCAL_COMMAND:-} ]] || exit 0
      [[ $(jq -r '.agent_id // empty' <<< "$input") == "" ]] || exit 0
      session=$(jq -r '.session_id // empty' <<< "$input")
      [[ -n $session ]] || exit 0
      setsid -f ${lib.getExe claudeResumeReport} "$session" </dev/null >/dev/null 2>&1
    '';
  };

  cyrillicKeys =
    let
      latin = lib.stringToCharacters "qwertyuiop[]asdfghjkl;'zxcvbnm,./";
      cyrillic = lib.splitString " " "й ц у к е н г ш щ з х ъ ф ы в а п р о л д ж э я ч с м и т ь б ю .";
    in
    assert lib.assertMsg (
      lib.length latin == lib.length cyrillic
    ) "herdr: keyboard layout rows must line up";
    lib.listToAttrs (lib.zipListsWith lib.nameValuePair latin cyrillic);

  withCyrillic =
    binds:
    let
      twin =
        bind:
        let
          parts = lib.splitString "+" bind;
          key = lib.last parts;
        in
        if cyrillicKeys ? ${key} then
          lib.concatStringsSep "+" (lib.init parts ++ [ cyrillicKeys.${key} ])
        else
          null;
      list = lib.toList binds;
      twins = lib.filter (bind: bind != null) (map twin list);
    in
    if twins == [ ] then binds else list ++ twins;

  keymap = lib.mapAttrs (_: binds: mkDefault (withCyrillic binds));

  ctxRules =
    let
      colors = {
        inherit (config.lib.stylix.colors.withHashtag) base08 base0A;
      };
      tone =
        tens:
        if tens >= 8 then
          {
            fg = colors.base08;
            bold = true;
          }
        else
          { fg = colors.base0A; };
      tens = lib.range 5 9;
    in
    map (d: { equals = "${toString d}%"; }) tens
    ++ [ ({ equals = "100%"; } // tone 10) ]
    ++ map (d: { starts_with = toString d; } // tone d) tens;

  agentRows = label: [
    [
      "state_icon"
      "workspace"
      "tab"
    ]
    [
      label
      {
        token = "$ctx";
        rules = ctxRules;
      }
      "$busy"
    ]
  ];

  phoneCfg = cfg.integrations.claude.phoneNotifications;

  substitute =
    name: substitutions:
    builtins.replaceStrings (builtins.attrNames substitutions) (builtins.attrValues substitutions) (
      builtins.readFile ./${name}.sh
    );

  mkScript =
    name: substitutions: pkgs.writeShellScript "herdr-claude-${name}" (substitute name substitutions);

  serverCheck = pkgs.writeShellScript "herdr-server-check" (
    substitute "server-check" {
      "@herdr@" = lib.getExe pkgs.herdr;
      "@timeout@" = "${pkgs.coreutils}/bin/timeout";
      "@sleep@" = "${pkgs.coreutils}/bin/sleep";
    }
  );

  launcher = pkgs.symlinkJoin {
    name = "herdr-launcher-${pkgs.herdr.version}";
    paths = [ pkgs.herdr ];
    postBuild = ''
      rm $out/bin/herdr
      install -m755 ${
        pkgs.writeShellScript "herdr-launcher" (
          substitute "launcher" {
            "@herdr@" = lib.getExe pkgs.herdr;
            "@systemctl@" = lib.getExe' pkgs.systemd "systemctl";
          }
        )
      } $out/bin/herdr
    '';
    inherit (pkgs.herdr) meta;
  };

  busyScript =
    name:
    mkScript name {
      "@herdr@" = lib.getExe pkgs.herdr;
      "@jq@" = lib.getExe pkgs.jq;
      "@date@" = "${pkgs.coreutils}/bin/date";
    };

  busyStart = busyScript "busy-start";
  busyTick = busyScript "busy-tick";
  busyStop = busyScript "busy-stop";
  statuslineReport = busyScript "statusline-report";

  phoneNotify = mkScript "phone-notify" {
    "@herdr@" = lib.getExe pkgs.herdr;
    "@jq@" = lib.getExe pkgs.jq;
    "@curl@" = lib.getExe pkgs.curl;
    "@basename@" = "${pkgs.coreutils}/bin/basename";
    "@endpoint@" = phoneCfg.endpoint;
    "@tokenFile@" = phoneCfg.tokenFile;
    "@onlyWhenDark@" = if phoneCfg.onlyWhenDark then "1" else "0";
  };

in
{
  options.${namespace}.programs.herdr = {
    enable = mkEnableOption "managing herdr";

    integrations = {
      claude = {
        enable = mkOption {
          type = types.bool;
          default = config.${namespace}.programs.claude-code.enable;
          defaultText = "config.\${namespace}.programs.claude-code.enable";
          description = "Whether to report Claude Code sessions to herdr.";
        };

        phoneNotifications = {
          enable = mkEnableOption "forwarding Claude Code attention events to the phone" // {
            default = osConfig.${namespace}.programs.haven.enable;
            defaultText = lib.literalExpression "osConfig.\${namespace}.programs.haven.enable";
          };

          endpoint = mkOption {
            type = types.str;
            default = "http://127.0.0.1:8730/mcp";
            description = "Haven MCP endpoint reachable through its reverse tunnel.";
          };

          tokenFile = mkOption {
            type = types.nullOr types.str;
            default = osConfig.clan.core.vars.generators.haven-mcp.files.token.path or null;
            defaultText = "the haven-mcp clan var";
            description = "File holding the Haven pairing token.";
          };

          onlyWhenDark = mkOption {
            type = types.bool;
            default = true;
            description = "Whether to notify only while every connected output is dark.";
          };
        };
      };
    };
  };

  config = mkIf cfg.enable (mkMerge [
    {
      systemd.user.services.herdr = {
        Unit = {
          Description = "herdr server";
          After = [ "graphical-session.target" ];
          X-SwitchMethod = "keep-old";
        };
        Service = {
          ExecCondition = "${serverCheck} absent";
          ExecStart = "${lib.getExe pkgs.herdr} server";
          ExecStartPost = "${serverCheck} ready";
          KillMode = "mixed";
          OOMPolicy = "continue";
          Restart = "on-failure";
        };
        Install.WantedBy = [ "graphical-session.target" ];
      };

      programs.herdr = {
        enable = true;
        package = launcher;
        settings = {
          onboarding = mkDefault false;
          update.version_check = mkDefault false;
          theme = {
            name = mkDefault "terminal";
            # keeps selected and active sidebar rows visible against the panel
            custom = with config.lib.stylix.colors.withHashtag; {
              accent = mkDefault base09;
              surface0 = mkDefault base03;
              surface1 = mkDefault base04;
              surface_dim = mkDefault base02;
            };
          };

          experimental.pane_history = mkDefault true;

          ui = {
            toast.delivery = mkDefault "herdr";
            sound = {
              done_path = mkDefault "${quietSounds}/done.mp3";
              request_path = mkDefault "${quietSounds}/request.mp3";
            };
            mobile_width_threshold = mkDefault 96;
            agent_panel_sort = mkDefault "priority";
            status_indicators = mkDefault "symbols";
            window_title = mkDefault "{hostname}: {workspace} · {terminal_title}";

            tab_bar_right = mkDefault [
              { type = "zoom"; }
              { type = "hostname"; }
            ];

            sidebar.agents = {
              rows = mkDefault (agentRows "agent");
              rows_by_agent.claude = mkDefault (agentRows "terminal_title_stripped");
            };
          };

          keys = keymap {
            focus_pane_left = "ctrl+alt+left";
            focus_pane_down = "ctrl+alt+down";
            focus_pane_up = "ctrl+alt+up";
            focus_pane_right = "ctrl+alt+right";
            previous_tab = [
              "ctrl+alt+["
              "ctrl+alt+y"
            ];
            next_tab = [
              "ctrl+alt+]"
              "ctrl+alt+i"
            ];
            previous_workspace = "ctrl+alt+shift+[";
            next_workspace = "ctrl+alt+shift+]";
            new_tab = "ctrl+alt+enter";
            split_vertical = "ctrl+alt+d";
            split_horizontal = "ctrl+alt+shift+d";
            zoom = "ctrl+alt+z";
            resize_mode = "ctrl+alt+r";
            next_agent = "ctrl+alt+a";
            previous_agent = "ctrl+alt+shift+a";
            focus_agent = "ctrl+alt+1..9";
            workspace_picker = "ctrl+alt+w";
            goto = "ctrl+alt+g";
            detach = "ctrl+alt+q";
            toggle_sidebar = "ctrl+alt+b";
            cycle_pane_next = "ctrl+alt+tab";
            cycle_pane_previous = "ctrl+alt+shift+tab";
            close_pane = "ctrl+alt+x";
            rename_pane = "ctrl+alt+p";
            close_tab = "ctrl+alt+shift+x";
            rename_tab = "ctrl+alt+shift+t";
            new_workspace = "ctrl+alt+n";
            close_workspace = "ctrl+alt+shift+n";
            rename_workspace = "ctrl+alt+shift+w";
            new_worktree = "ctrl+alt+shift+g";
            open_worktree = "ctrl+alt+o";
            remove_worktree = "ctrl+alt+shift+o";
            edit_scrollback = "ctrl+alt+e";
            copy_mode = "ctrl+alt+c";
            help = "ctrl+alt+h";
            settings = "ctrl+alt+s";
            open_notification_target = "ctrl+alt+m";
            reload_config = "ctrl+alt+shift+r";
          };
        };
      };
    }

    (mkIf config.programs.lazygit.enable {
      programs.herdr.settings.keys.command = [
        {
          key = withCyrillic "ctrl+alt+l";
          type = "popup";
          command = ''cd "$HERDR_ACTIVE_PANE_CWD" && exec ${lib.getExe config.programs.lazygit.package}'';
          description = "lazygit";
          width = "90%";
          height = "90%";
        }
      ];
    })

    (mkIf cfg.integrations.claude.enable {
      ${namespace}.programs.claude-code.statusLineHooks = [ statuslineReport ];

      programs.claude-code.skills.herdr = "${pkgs.herdr.src}/skills/herdr";

      programs.claude-code.settings.hooks = {
        SessionStart = [
          {
            matcher = "*";
            hooks = [
              {
                type = "command";
                command = "${claudeHook} session";
                timeout = 10;
              }
              {
                type = "command";
                command = lib.getExe claudeResumeHook;
                timeout = 5;
              }
            ];
          }
        ];

        UserPromptSubmit = [
          {
            hooks = [
              {
                type = "command";
                command = "${busyStart}";
                timeout = 5;
              }
            ];
          }
        ];

        PostToolUse = [
          {
            matcher = "*";
            hooks = [
              {
                type = "command";
                command = "${busyTick}";
                timeout = 5;
              }
            ];
          }
        ];

        Stop = [
          {
            hooks = [
              {
                type = "command";
                command = "${busyStop}";
                timeout = 5;
              }
            ];
          }
        ];

        Notification = lib.optional phoneCfg.enable {
          matcher = "permission_prompt|agent_needs_input|idle_prompt";
          hooks = [
            {
              type = "command";
              command = "${phoneNotify}";
              timeout = 15;
            }
          ];
        };
      };
    })
  ]);
}

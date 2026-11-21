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
    mkBefore
    mkDefault
    mkForce
    mkEnableOption
    mkOption
    types
    attrNames
    filterAttrs
    last
    mapAttrs
    mapAttrsToList
    splitString
    ;
  cfg = config.${namespace}.programs.noctalia;
  tomlFormat = pkgs.formats.toml { };

  pluginSource =
    id: plugin:
    if plugin.patches == [ ] then
      plugin.src
    else
      pkgs.applyPatches {
        name = "${last (splitString "/" id)}-plugin-patched";
        inherit (plugin) src patches;
      };

  noctaliaPlugin =
    repo: name: rev: hash:
    "${
      pkgs.fetchFromGitHub {
        owner = "noctalia-dev";
        inherit repo rev hash;
        sparseCheckout = [ name ];
      }
    }/${name}";

  pluginDir = pkgs.linkFarm "noctalia-plugins" (
    mapAttrsToList (id: plugin: {
      name = last (splitString "/" id);
      path = pluginSource id plugin;
    }) cfg.plugins
  );

  package = pkgs.noctalia.overrideAttrs (old: {
    postFixup = old.postFixup + ''
      wrapProgram $out/bin/noctalia \
        --suffix PATH : ${lib.makeBinPath cfg.runtimeInputs}
    '';
  });

  msg = command: "noctalia msg ${command}";
  lockedMsg = command: {
    action.spawn-sh = msg command;
    allow-when-locked = true;
  };

  idlePowerProfile = pkgs.writeShellScript "idle-power-profile" ''
    state="$XDG_RUNTIME_DIR/idle-power-profile"
    case "$1" in
      enter)
        [ -e "$state" ] || powerprofilesctl get > "$state"
        powerprofilesctl set power-saver
        ;;
      leave)
        [ -s "$state" ] && powerprofilesctl set "$(cat "$state")"
        rm -f "$state"
        ;;
    esac
  '';
in
{
  options.${namespace}.programs.noctalia = {
    enable = mkEnableOption "managing Noctalia shell";

    plugins = mkOption {
      type = types.attrsOf (
        types.submodule {
          options = {
            src = mkOption {
              type = types.path;
              description = "Plugin directory containing plugin.toml.";
            };
            patches = mkOption {
              type = types.listOf types.path;
              default = [ ];
              description = "Patches applied to the plugin directory.";
            };
            settings = mkOption {
              inherit (tomlFormat) type;
              default = { };
              description = "Plugin settings written to `plugin_settings`.";
            };
          };
        }
      );
      default = { };
      description = "Plugins to install and enable, keyed by plugin id.";
    };

    runtimeInputs = mkOption {
      type = types.listOf types.package;
      default = [ ];
      description = "Packages added to the PATH of the shell and its plugins.";
    };
  };

  config = mkIf cfg.enable {
    stylix.targets.noctalia.fonts.enable = false;

    programs.noctalia = {
      enable = true;
      inherit package;
      settings = {
        shell = {
          setup_wizard_enabled = false;
          font_family = config.stylix.fonts.monospace.name;
          time_format = "{:%H:%M:%S}";
          clipboard_history_max_entries = 500;
          launch_apps_custom_command = "${pkgs.uwsm}/bin/uwsm-app -- $CMD";
          panel.open_near_click_control_center = true;
          launcher = {
            layout = "simple";
            category_icons_only = true;
          };
          screenshot = {
            directory = "~/Pictures/Screenshots";
            save_to_file = false;
            annotate = true;
            skip_annotate_on_copy_save = true;
          };
          session.actions = [
            {
              action = "lock_and_suspend";
              shortcut = "2";
            }
            {
              action = "reboot";
              shortcut = "4";
            }
            {
              action = "logout";
              shortcut = "5";
            }
            {
              action = "shutdown";
              variant = "destructive";
              shortcut = "6";
            }
          ];
        };

        bar.main = {
          thickness = 36;
          padding = 8;
          margin_ends = 0;
          radius_top_left = 0;
          radius_top_right = 0;
          capsule = true;
          capsule_opacity = 0.7;
          widget_spacing = 4;
          start = [
            "caffeine"
            "workspaces"
            "cpu"
            "cpu_temp"
            "ram"
            "net_rx"
            "net_tx"
            "active_window"
          ];
          center = [
            "weather"
            "clock"
            "media"
          ];
          end = [
            "tray"
            "keyboard_layout"
            "udiskie"
            "tailnet"
            "hassio"
            "notifications"
            "screen_recorder"
            "battery"
            "volume"
            "brightness"
            "control-center"
          ];
        };

        widget = {
          workspaces = {
            focused_color = "primary";
            occupied_color = "#00000000";
            empty_color = "#00000000";
          };
          cpu = {
            type = "sysmon";
            stat = "cpu_usage";
          };
          cpu_temp = {
            type = "sysmon";
            stat = "cpu_temp";
          };
          ram = {
            type = "sysmon";
            stat = "ram_used";
          };
          net_rx = {
            type = "sysmon";
            stat = "net_rx";
          };
          net_tx = {
            type = "sysmon";
            stat = "net_tx";
          };
          active_window.max_length = 350;
          clock = {
            anchor = true;
            format = "{:%H:%M %a %d/%m/%Y}";
          };
          keyboard_layout.show_glyph = false;
          media = {
            min_length = 48;
            max_length = 350;
            hide_artist = true;
            hide_when_no_media = true;
          };
          udiskie.type = "aristides/udiskie:status";
          tailnet = {
            type = "rylos/tailnet:bar";
            show_count = false;
          };
          hassio.type = "pozzoo/hassio:status";
          screen_recorder.type = "noctalia/screen_recorder:recorder";
        };

        backdrop.enabled = mkDefault true;
        location.auto_locate = true;
        brightness.enable_ddcutil = osConfig.${namespace}.system.hardware.ddc.enable;
        control_center = {
          sidebar = "full";
          sidebar_section = "full";
        };

        osd = {
          background_opacity = mkForce 0.7;
          follow_focused_output = true;
          hide_delay_ms = 850;
          kinds.media = false;
        };

        notification = {
          history_retention_hours = 8;
          max_visible = 3;
        };

        lockscreen = {
          allow_empty_password = osConfig.${namespace}.system.hardware.yubikey.enable;
          grace_period_seconds = 3;
          transition = [ "wipe" ];
          transition_duration = 800;
        };

        idle = {
          pre_action_fade_seconds = 0;
          behavior = {
            dim = {
              timeout = 540;
              action = "command";
              command = "brightnessctl set 20%- --save";
              resume_command = "brightnessctl --restore";
            };
            lock = {
              timeout = 600;
              action = "lock";
            };
            screen-off = {
              timeout = 1200;
              action = "screen_off";
            };
            power-saver = {
              timeout = 1210;
              action = "command";
              command = "${idlePowerProfile} enter";
              resume_command = "${idlePowerProfile} leave";
            };
          };
        };

        plugins = {
          enabled = attrNames cfg.plugins;
          auto_update = "none";
          source = [
            {
              name = "nix";
              kind = "path";
              location = "${pluginDir}";
            }
          ];
        };

        plugin_settings = mapAttrs (_: plugin: plugin.settings) (
          filterAttrs (_: plugin: plugin.settings != { }) cfg.plugins
        );
      };
    };

    ${namespace}.programs.noctalia = {
      plugins = {
        "noctalia/screen_recorder" = {
          src =
            noctaliaPlugin "official-plugins" "screen_recorder" "a9fe0e04775c26efcd9f21df6a499d5c355f4cdf"
              "sha256-5NWh1QmqZgrKcwiOKuQ9vO5S2dV7excqutWnm3KAw/8=";
          settings = {
            directory = "${config.home.homeDirectory}/Videos/Recordings";
            copy_to_clipboard = true;
            frame_rate = 144;
            video_codec = "hevc";
            color_range = "full";
            replay_enabled = true;
            replay_duration = 60;
          };
        };
        "rylos/tailnet".src =
          noctaliaPlugin "community-plugins" "tailnet" "ea9e1016be3348d6b8517ad164ed6b334f200bdc"
            "sha256-O8G+T0s2yVoPdJWG3sppot7+eWAnSj5HGzgrz2P8LPs=";
        "cleboost/zed-provider".src =
          noctaliaPlugin "community-plugins" "zed-provider" "693ddaf2cfc5dc3678d4e95aaa12e51b28655ef4"
            "sha256-UDwncc4XjOkwUAvJErykjRdOhmB8gkUZSsx17vtSgxk=";
        "aristides/udiskie" = {
          src =
            noctaliaPlugin "community-plugins" "udiskie" "57e4b108c44a30ae2449b50feb81fc8f512a5cf7"
              "sha256-4u3Wd1W3CFy03Qs/fGKfJSU19UHBMRSEp8UaBD6HSx0=";
          patches = [ ./patches/noctalia-udiskie-fetch-retry.patch ];
          settings.file_manager_cmd = "thunar";
        };
        "pozzoo/hassio".src =
          noctaliaPlugin "community-plugins" "hassio" "693ddaf2cfc5dc3678d4e95aaa12e51b28655ef4"
            "sha256-aoIYNF5SPPtg9ZpJGVQMSHSap/0vVfg7rTLrLpYVyZ0=";
      };

      runtimeInputs = with pkgs; [
        glib
        gpu-screen-recorder
        slurp
        sqlite
        udiskie
        xdg-user-dirs
        xdg-utils
      ];
    };

    programs.niri.settings = mkIf config.${namespace}.desktop-environments.niri.enable {
      binds = {
        "Mod+D" = {
          action.spawn-sh = msg "panel-toggle launcher";
          hotkey-overlay.title = "Open Application Launcher (Noctalia)";
        };
        "Mod+Shift+C" = {
          action.spawn-sh = msg "panel-toggle clipboard";
          hotkey-overlay.title = "Open Clipboard History (Noctalia)";
        };
        "Mod+Shift+L" = {
          action.spawn-sh = msg "session lock";
          hotkey-overlay.title = "Lock the Screen (Noctalia)";
        };
        "Mod+Shift+I" = {
          action.spawn-sh = msg "caffeine-toggle";
          hotkey-overlay.title = "Toggle Idle Inhibitor (Noctalia)";
        };
        "Print" = {
          action.spawn-sh = msg "screenshot-region";
          hotkey-overlay.title = "Screenshot a Region (Noctalia)";
        };
        "Ctrl+Print" = {
          action.spawn-sh = msg "screenshot-fullscreen";
          hotkey-overlay.title = "Screenshot the Screen (Noctalia)";
        };
        "Shift+Print" = {
          action.spawn-sh = msg "annotate";
          hotkey-overlay.title = "Draw over the Screen (Noctalia)";
        };
        "Mod+Ctrl+Shift+Print" = {
          action.spawn-sh = msg "plugin noctalia/screen_recorder:service all toggle";
          hotkey-overlay.title = "Toggle Screen Recording (Noctalia)";
        };

        "XF86AudioRaiseVolume" = lockedMsg "volume-up";
        "XF86AudioLowerVolume" = lockedMsg "volume-down";
        "XF86AudioMute" = lockedMsg "volume-mute";
        "XF86AudioMicMute" = lockedMsg "mic-mute";
        "XF86MonBrightnessUp" = lockedMsg "brightness-up";
        "XF86MonBrightnessDown" = lockedMsg "brightness-down";
      };

      layer-rules = mkBefore [
        {
          matches = [ { namespace = "^noctalia-notification$"; } ];
          block-out-from = "screencast";
        }
        {
          matches = [ { namespace = "^noctalia-backdrop$"; } ];
          place-within-backdrop = true;
        }
      ];

      spawn-at-startup = [
        { command = [ "noctalia" ]; }
      ];
    };
  };
}

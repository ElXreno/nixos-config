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
    mkDefault
    genAttrs
    optionalString
    ;
  cfg = config.${namespace}.desktop-environments.niri;
  spotify = config.${namespace}.programs.spicetify.enable;

  uwsm-app = "${pkgs.uwsm}/bin/uwsm-app --";

  workspaces = map toString (lib.range 1 9);
  workspaceBinds =
    prefix: action:
    lib.mapAttrs' (n: _: lib.nameValuePair "${prefix}${n}" { action.${action} = lib.toInt n; }) (
      genAttrs workspaces (_: null)
    );

  player = command: {
    action.spawn-sh = "playerctl ${command}${optionalString spotify " --player=spotify"}";
    allow-when-locked = true;
  };
in
{
  config = mkIf cfg.enable {
    programs.niri.settings.binds = {
      "Mod+Shift+Slash".action.show-hotkey-overlay = [ ];

      "Mod+B" = {
        action.spawn-sh = "${uwsm-app} firefox";
        hotkey-overlay.title = "Open a Browser: firefox";
      };
      "Mod+Return" = {
        action.spawn-sh = "${uwsm-app} kitty";
        hotkey-overlay.title = "Open a Terminal: kitty";
      };
      "Mod+E" = {
        action.spawn-sh = "${uwsm-app} thunar";
        hotkey-overlay.title = "Run an File Manager: thunar";
      };

      "XF86AudioPlay" = player "play-pause";
      "XF86AudioPause" = player "play-pause";
      "XF86AudioStop" = player "pause";
      "XF86AudioPrev" = player "previous";
      "XF86AudioNext" = player "next";

      "Mod+Tab" = {
        action.toggle-overview = [ ];
        repeat = false;
      };

      "Mod+Q" = {
        action.close-window = [ ];
        repeat = false;
      };

      "Mod+Left".action.focus-column-left = [ ];
      "Mod+Down".action.focus-window-down = [ ];
      "Mod+Up".action.focus-window-up = [ ];
      "Mod+Right".action.focus-column-right = [ ];

      "Mod+Ctrl+Left".action.move-column-left = [ ];
      "Mod+Ctrl+Down".action.move-window-down = [ ];
      "Mod+Ctrl+Up".action.move-window-up = [ ];
      "Mod+Ctrl+Right".action.move-column-right = [ ];

      "Mod+Home".action.focus-column-first = [ ];
      "Mod+End".action.focus-column-last = [ ];
      "Mod+Ctrl+Home".action.move-column-to-first = [ ];
      "Mod+Ctrl+End".action.move-column-to-last = [ ];

      "Mod+Shift+Left".action.focus-monitor-left = [ ];
      "Mod+Shift+Down".action.focus-monitor-down = [ ];
      "Mod+Shift+Up".action.focus-monitor-up = [ ];
      "Mod+Shift+Right".action.focus-monitor-right = [ ];

      "Mod+Shift+Ctrl+Left".action.move-column-to-monitor-left = [ ];
      "Mod+Shift+Ctrl+Down".action.move-column-to-monitor-down = [ ];
      "Mod+Shift+Ctrl+Up".action.move-column-to-monitor-up = [ ];
      "Mod+Shift+Ctrl+Right".action.move-column-to-monitor-right = [ ];

      "Mod+Page_Down".action.focus-workspace-down = [ ];
      "Mod+Page_Up".action.focus-workspace-up = [ ];
      "Mod+Ctrl+Page_Down".action.move-column-to-workspace-down = [ ];
      "Mod+Ctrl+Page_Up".action.move-column-to-workspace-up = [ ];

      "Mod+Shift+Page_Down".action.move-workspace-down = [ ];
      "Mod+Shift+Page_Up".action.move-workspace-up = [ ];

      "Mod+WheelScrollDown" = {
        action.focus-workspace-down = [ ];
        cooldown-ms = 50;
      };
      "Mod+WheelScrollUp" = {
        action.focus-workspace-up = [ ];
        cooldown-ms = 50;
      };

      "Mod+Shift+WheelScrollDown".action.focus-column-right = [ ];
      "Mod+Shift+WheelScrollUp".action.focus-column-left = [ ];

      "Mod+BracketLeft".action.consume-or-expel-window-left = [ ];
      "Mod+BracketRight".action.consume-or-expel-window-right = [ ];

      "Mod+Comma".action.consume-window-into-column = [ ];
      "Mod+Period".action.expel-window-from-column = [ ];

      "Mod+R".action.switch-preset-column-width = [ ];
      "Mod+Shift+R".action.switch-preset-window-height = [ ];
      "Mod+Ctrl+R".action.reset-window-height = [ ];
      "Mod+F".action.maximize-column = [ ];
      "Mod+Shift+F".action.fullscreen-window = [ ];

      "Mod+Ctrl+F".action.expand-column-to-available-width = [ ];

      "Mod+C".action.center-column = [ ];

      "Mod+Ctrl+C".action.center-visible-columns = [ ];

      "Mod+Minus".action.set-column-width = "-10%";
      "Mod+Equal".action.set-column-width = "+10%";

      "Mod+Shift+Minus".action.set-window-height = "-10%";
      "Mod+Shift+Equal".action.set-window-height = "+10%";

      "Mod+V".action.toggle-window-floating = [ ];
      "Mod+Shift+V".action.switch-focus-between-floating-and-tiling = [ ];

      "Mod+W".action.toggle-column-tabbed-display = [ ];

      "Print" = mkDefault { action.screenshot = [ ]; };
      "Ctrl+Print" = mkDefault { action.screenshot-screen = [ ]; };
      "Alt+Print".action.screenshot-window = {
        write-to-disk = false;
      };

      "Mod+Escape" = {
        action.toggle-keyboard-shortcuts-inhibit = [ ];
        allow-inhibiting = false;
      };

      "Ctrl+Alt+Delete".action.quit = [ ];

      "Mod+Shift+P".action.power-off-monitors = [ ];
    }
    // workspaceBinds "Mod+" "focus-workspace"
    // workspaceBinds "Mod+Ctrl+" "move-column-to-workspace"
    // lib.optionalAttrs osConfig.services.ryukaze.enable {
      "Mod+Shift+B" = {
        action.spawn = [
          "ryukaze"
          "--toggle-local-dimming"
        ];
        hotkey-overlay.title = "Toggle Local Dimming (ryukaze)";
      };
      "XF86Tools" = {
        action.spawn = [
          "ryukaze"
          "--next-scenario"
        ];
        hotkey-overlay.title = "Next Scenario (ryukaze)";
      };
    };
  };
}

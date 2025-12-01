{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.hyprland;
in
{
  options.${namespace}.desktop-environments.hyprland = {
    enable = mkEnableOption "Whether or not to manage hyprland.";
  };

  config = mkIf cfg.enable {
    ${namespace}.programs = {
      hypridle.enable = true;
      noctalia.enable = true;
    };

    services.hyprpolkitagent.enable = true;
    services.playerctld.enable = true;

    home.packages = with pkgs; [
      xfce.thunar
      xfce.tumbler
      qimgv
      pavucontrol
      wl-clipboard
      cliphist
      wofi
      hyprcursor
      hyprshot
    ];

    home.pointerCursor = {
      name = "Breeze_Hacked";
      size = 24;
      package = pkgs.breeze-hacked-cursor-theme;

      enable = true;

      x11.enable = true;
      gtk.enable = true;
      hyprcursor = {
        enable = true;
      };
    };
    programs.kitty.enable = true; # required for the default Hyprland config

    xdg.mimeApps.defaultApplications = {
      "inode/directory" = "thunar.desktop";
    };

    wayland.windowManager.hyprland = {
      enable = true;
      systemd.enable = false;

      package = null;
      portalPackage = null;

      plugins = with pkgs.${namespace}; [
        split-monitor-workspaces
      ];

      settings = {
        "$mod" = "SUPER";
        "$terminal" = lib.getExe pkgs.kitty;
        "$menu" = "wofi -S drun";
        "$fileManager" = "thunar";
        "$editor" = "zeditor";

        input = {
          kb_layout = "us,ru";
          kb_options = "grp:win_space_toggle";

          accel_profile = "flat";

          touchpad = {
            natural_scroll = true;
            middle_button_emulation = true;
          };

          numlock_by_default = true;
        };

        gestures = {
          workspace_swipe_distance = 300;
          workspace_swipe_invert = true;
          workspace_swipe_min_speed_to_force = 30;
          workspace_swipe_forever = true;
        };

        device = {
          name = "lamzu-lamzu-maya-x-8k-dongle-1";
          middle_button_emulation = false;
          sensitivity = 0;
        };

        gesture = [ "3, horizontal, workspace" ];

        monitor = [
          "eDP-1, 1920x1080@144, 0x0, 1"
          "eDP-2, 1920x1080@144, 0x0, 1, vrr, 2"
          "DP-2, preferred, -1920x0, 1.0"
          "HDMI-A-1, 1920x1080@60, -1920x0, 1.0"
        ];

        cursor = {
          default_monitor = "eDP-1";
        };

        render = {
          direct_scanout = 2;
          new_render_scheduling = true;
        };

        general = {
          allow_tearing = true;

          gaps_out = 16;

          "col.active_border" = "rgb(F3CC26)";
        };

        animations = {
          enabled = true;
          bezier = "swift, 0.2, 0.0, 0.0, 1.0";

          animation = "workspaces, 1, 4, swift, fade";
        };

        decoration = {
          rounding = 4;
          shadow = {
            enabled = true;
          };

          rounding_power = 4.0;

          active_opacity = 0.9;
          inactive_opacity = 0.8;
          fullscreen_opacity = 1.0;

          dim_inactive = true;
          dim_strength = 0.03;
        };

        dwindle = {
          force_split = 2;
          preserve_split = true;
        };

        plugin = {
          split-monitor-workspaces = {
            count = 10;
          };
        };

        bind = [
          ", Print, exec, hyprshot -zm region --clipboard-only"
          "$mod, B, exec, firefox"
          "$mod, Return, exec, $terminal"
          "$mod, R, exec, noctalia-shell ipc call launcher toggle"
          "$mod, E, exec, $fileManager"
          "$mod, D, exec, $editor"
          "$mod, H, exec, noctalia-shell ipc call launcher clipboard"
          "$mod, L, exec, loginctl lock-session"

          "$mod, M, exit,"
          "$mod, Q, killactive,"
          "$mod, J, layoutmsg, togglesplit"
          "$mod SHIFT, J, layoutmsg, swapsplit"

          "$mod, left, movefocus, l"
          "$mod, right, movefocus, r"
          "$mod, up, movefocus, u"
          "$mod, down, movefocus, d"

          "$mod SHIFT, left, movewindow, l"
          "$mod SHIFT, right, movewindow, r"
          "$mod SHIFT, up, movewindow, u"
          "$mod SHIFT, down, movewindow, d"

          "$mod ALT, left, resizeactive, -20 0"
          "$mod ALT, right, resizeactive, 20 0"
          "$mod ALT, up, resizeactive, 0 -20"
          "$mod ALT, down, resizeactive, 0 20"
        ]
        ++ (builtins.concatLists (
          builtins.genList (
            i:
            let
              ws = i + 1;
            in
            [
              "$mod, code:${toString (10 + i)}, split-workspace, ${toString ws}"
              "$mod SHIFT, code:${toString (10 + i)}, split-movetoworkspace, ${toString ws}"
            ]
          ) 10
        ));

        bindle = [
          ", XF86MonBrightnessUp,   exec, noctalia-shell ipc call brightness increase"
          ", XF86MonBrightnessDown, exec, noctalia-shell ipc call brightness decrease"

          ", XF86AudioRaiseVolume, exec, noctalia-shell ipc call volume increase"
          ", XF86AudioLowerVolume, exec, noctalia-shell ipc call volume decrease"
        ];

        bindl = [
          ", XF86AudioMute,    exec, noctalia-shell ipc call volume muteOutput"
          ", XF86AudioMicMute, exec, noctalia-shell ipc call volume muteInput"
          ", XF86AudioPlay,  exec, noctalia-shell ipc call media play"
          ", XF86AudioPause, exec, noctalia-shell ipc call media pause"
          ", XF86AudioPrev,  exec, noctalia-shell ipc call media next"
          ", XF86AudioNext,  exec, noctalia-shell ipc call media previous"

          ", switch:on:Lid Switch,  exec, hyprctl keyword monitor 'eDP-1, disable'"
          ", switch:off:Lid Switch, exec, hyprctl keyword monitor 'eDP-1, preffered, auto, 1'"
        ];

        windowrule = [
          "float on, match:class ^(nz.co.mega.)$"
          "border_size 0, match:class ^(nz.co.mega.)$"
          "stay_focused on, match:class ^(nz.co.mega.)$"

          "float on, match:class ^(steam)$, match:title ^(Friends List)$"
          "float on, match:class ^(org.freedesktop.impl.portal.desktop.kde)$"

          "opacity 0.0 override, match:class ^(xwaylandvideobridge)$"
          "no_anim on, match:class ^(xwaylandvideobridge)$"
          "no_focus on, match:class ^(xwaylandvideobridge)$"
          "no_initial_focus on, match:class ^(xwaylandvideobridge)$"

          "workspace 1, match:class ^(firefox)$"
          "workspace 5, no_screen_share on, match:class ^(org.telegram.desktop)$"
          "workspace 9, match:class ^(spotify)$"
          "workspace 10, no_screen_share on, match:class ^(org.keepassxc.KeePassXC)$"
          "workspace 11, no_screen_share on, match:class ^(discord)$"
          "workspace special silent, match:class ^(explorer.exe)$"

          "immediate on, match:class ^(.*DDNet)$"
          "immediate on, match:title ^(Minecraft.*)$"
          "immediate on, match:initial_title ^(Minecraft.*)$"
          "immediate on, match:title ^(STALCRAFT)$"

          "suppress_event fullscreen maximize, match:class ^(exbolauncher\.exe|steam_proton)$"
        ];

        misc = {
          disable_hyprland_logo = true;
          disable_splash_rendering = true;
          force_default_wallpaper = 0;
          enable_anr_dialog = false;
        };

        xwayland = {
          force_zero_scaling = true;
        };

        exec-once = [
          "wl-paste --type text --watch cliphist store"
          "wl-paste --type image --watch cliphist store"
          (
            let
              monitor-hotplug = pkgs.writeShellScript "monitor-hotplug.sh" ''
                handle() {
                  case $1 in
                    monitoradded*|monitorremoved*) hyprctl dispatch focusmonitor eDP-1 ;;
                  esac
                }

                ${lib.getExe pkgs.socat} -U - UNIX-CONNECT:$XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE/.socket2.sock | while read -r line; do handle "$line"; done
              '';
            in
            monitor-hotplug
          )
        ];
      };
    };

    gtk = {
      enable = true;

      theme = {
        package = pkgs.flat-remix-gtk;
        name = "Flat-Remix-GTK-Grey-Darkest";
      };

      iconTheme = {
        package = pkgs.papirus-icon-theme;
        name = "Papirus-Dark";
      };

      font = {
        name = "Sans";
        size = 11;
      };
    };
  };
}

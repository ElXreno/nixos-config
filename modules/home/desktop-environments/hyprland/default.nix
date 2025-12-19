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
      hyprlock.enable = true;
      hyprpaper.enable = true;
      waybar.enable = true;
    };

    services.hyprpolkitagent.enable = true;
    services.dunst = {
      enable = true;

      settings = {
        global = {
          follow = "keyboard";
        };
      };
    };
    services.playerctld.enable = true;

    home.packages = with pkgs; [
      xfce.thunar
      xfce.tumbler
      qimgv
      waybar
      pavucontrol
      networkmanagerapplet
      blueman
      wl-clipboard
      cliphist
      wofi
      brightnessctl
      hyprcursor
      hyprshot
      playerctl
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
        hyprsplit
        hy3
      ];

      settings = {
        "$mod" = "SUPER";
        "$terminal" = "${pkgs.kitty}/bin/kitty";
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

          "DP-2, 2560x1440@180.00Hz, auto-center-right, 1.0, cm, dcip3"
        ];

        cursor = {
          default_monitor = "DP-2";
        };

        render = {
          direct_scanout = 2;
        };

        general = {
          allow_tearing = true;

          gaps_out = 16;
          "col.active_border" = "rgb(F3CC26)";

          layout = "hy3";
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

          active_opacity = 1.0;
          inactive_opacity = 1.0;
          fullscreen_opacity = 1.0;

          dim_inactive = true;
          dim_strength = 0.03;
        };

        dwindle = {
          force_split = 2;
          preserve_split = true;
        };

        plugin = {
          hyprsplit = {
            persistent_workspaces = true;
          };
          hy3 = {
            tabs.text_center = true;
            autotile.enable = true;
          };
        };

        bind = [
          ", Print, exec, hyprshot -zm region --clipboard-only"
          "$mod, B, exec, firefox"
          "$mod, Return, exec, $terminal"
          "$mod, R, exec, $menu"
          "$mod, E, exec, $fileManager"
          "$mod, D, exec, $editor"
          "$mod, H, exec, cliphist list | wofi -d | cliphist decode | wl-copy"
          "$mod, L, exec, hyprlock"

          "$mod, M, exit,"
          "$mod, Q, killactive,"

          "$mod, left, hy3:movefocus, l"
          "$mod, right, hy3:movefocus, r"
          "$mod, up, hy3:movefocus, u"
          "$mod, down, hy3:movefocus, d"

          "$mod SHIFT, left, hy3:movewindow, l"
          "$mod SHIFT, right, hy3:movewindow, r"
          "$mod SHIFT, up, hy3:movewindow, u"
          "$mod SHIFT, down, hy3:movewindow, d"

          "$mod SHIFT, bracketleft, movewindow, mon:l"
          "$mod SHIFT, bracketright, movewindow, mon:r"

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
              "$mod, code:${toString (10 + i)}, split:workspace, ${toString ws}"
              "$mod SHIFT, code:${toString (10 + i)}, split:movetoworkspace, ${toString ws}"
            ]
          ) 10
        ));

        bindle = [
          ", XF86MonBrightnessUp,   exec, brightnessctl set +5%"
          ", XF86MonBrightnessDown, exec, brightnessctl set 5%-"

          ", XF86AudioRaiseVolume, exec, wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%+ -l 1"
          ", XF86AudioLowerVolume, exec, wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%- -l 1"
        ];

        bindl = [
          ", XF86AudioMute,    exec, wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"
          ", XF86AudioMicMute, exec, wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle"
          ", XF86AudioPlay,  exec, playerctl play --player=spotify"
          ", XF86AudioPause, exec, playerctl pause --player=spotify"
          ", XF86AudioStop, exec, playerctl pause --player=spotify"
          ", XF86AudioPrev,  exec, playerctl previous --player=spotify"
          ", XF86AudioNext,  exec, playerctl next --player=spotify"

          # " , switch:Lid Switch,     exec, hyprlock" # Remove it if I will use suspend mode
          " , switch:on:Lid Switch,  exec, hyprctl keyword monitor 'eDP-1, disable'"
          " , switch:off:Lid Switch, exec, hyprctl keyword monitor 'eDP-1, 1920x1080@144, auto-center-left, 1'"
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

          "workspace 4, no_screen_share on, match:class ^(discord)$"
          "workspace 5, no_screen_share on, match:class ^(org.telegram.desktop)$"
          "workspace 9, match:class ^(spotify)$"
          "workspace 10, no_screen_share on, match:class ^(org.keepassxc.KeePassXC)$"
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
          "nm-applet --indicator"
          "blueman-applet"
          "wl-paste --type text --watch cliphist store"
          "wl-paste --type image --watch cliphist store"
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

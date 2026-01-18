{
  config,
  namespace,
  lib,
  pkgs,
  osConfig,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.niri;

  app2unit = "${lib.getExe pkgs.app2unit} --";
in
{
  options.${namespace}.desktop-environments.niri = {
    enable = mkEnableOption "Whether or not to manage niri.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      programs = {
        waybar.enable = true;
        hyprlock.enable = true;
        walker.enable = true;
        kitty.enable = true;
      };
      services = {
        hypridle.enable = true;
        mako.enable = true;
      };
    };

    services.network-manager-applet.enable = true;
    services.blueman-applet.enable = true;
    services.playerctld.enable = true;
    services.cliphist.enable = true;

    home.packages = with pkgs; [
      xwayland-satellite
      playerctl
      pavucontrol
      qimgv
      thunar
      tumbler
      cliphist
      wl-clipboard
    ];

    home.pointerCursor = {
      enable = true;

      name = "Breeze_Hacked";
      size = 24;
      package = pkgs.breeze-hacked-cursor-theme;

      x11.enable = true;
      gtk.enable = true;
    };

    home.sessionVariables = {
      QT_QPA_PLATFORM = "wayland;xcb";
      GDK_BACKEND = "wayland,x11";
      CLUTTER_BACKEND = "wayland";
      SDL_VIDEODRIVER = "wayland";

      NIXOS_OZONE_WL = 1;
      ELECTRON_OZONE_PLATFORM_HINT = "wayland"; # Fallback for NIXOS_OZONE_WL, non-nix packaged software
    };

    programs.niri.settings = {
      input = {
        keyboard = {
          xkb = {
            layout = "us,ru";
            options = "grp:win_space_toggle";
          };
          numlock = true;
        };

        mouse = {
          accel-profile = "flat";
          accel-speed = 0.0;
        };

        warp-mouse-to-focus.enable = true;

        focus-follows-mouse = {
          enable = true;
          max-scroll-amount = "0%";
        };
      };

      outputs = {
        "eDP-1" = {
          position = {
            x = 0;
            y = 360;
          };
        };
        "Xiaomi Corporation Mi monitor 5392700044842" = {
          mode = {
            width = 2560;
            height = 1440;
            refresh = 180.0;
          };
          focus-at-startup = true;
        };
      };

      layout = {
        gaps = 12;
        background-color = "transparent";

        focus-ring.enable = false;
        border = {
          enable = true;
          width = 1;
          active.color = "#cccccc";
          inactive.color = "#505050";
          urgent.color = "#9b0000";
        };

        shadow = {
          enable = true;
          softness = 20.0;
          spread = 1;
          offset = {
            x = 2;
            y = 2;
          };
          color = "#222222";
        };
      };

      binds = {
        "Mod+Shift+Slash".action.show-hotkey-overlay = [ ];

        "Mod+B" = {
          action.spawn-sh = "${app2unit} firefox";
          hotkey-overlay.title = "Open a Browser: firefox";
        };
        "Mod+Return" = {
          action.spawn-sh = "${app2unit} kitty";
          hotkey-overlay.title = "Open a Terminal: kitty";
        };
        "Mod+D" =
          let
            uid = osConfig.users.users.${config.home.username}.uid;
          in
          {
            action.spawn-sh = "nc -U /run/user/${toString uid}/walker/walker.sock";
            hotkey-overlay.title = "Run an Application: walker";
          };
        "Mod+E" = {
          action.spawn-sh = "${app2unit} thunar";
          hotkey-overlay.title = "Run an File Manager: thunar";
        };
        "Mod+Shift+D" = {
          action.spawn-sh = "${app2unit} zeditor";
          hotkey-overlay.title = "Run an Text Editor: zeditor";
        };
        "Mod+Alt+L" = {
          action.spawn-sh = "${app2unit} hyprlock";
          hotkey-overlay.title = "Lock the Screen: hyprlock";
        };

        "XF86AudioRaiseVolume" = {
          action.spawn-sh = "wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%+ -l 1";
          allow-when-locked = true;
        };
        "XF86AudioLowerVolume" = {
          action.spawn-sh = "wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%- -l 1";
          allow-when-locked = true;
        };
        "XF86AudioMute" = {
          action.spawn-sh = "wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle";
          allow-when-locked = true;
        };
        "XF86AudioMicMute" = {
          action.spawn-sh = "wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle";
          allow-when-locked = true;
        };

        "XF86AudioPlay" = {
          action.spawn-sh = "playerctl play --player=spotify";
          allow-when-locked = true;
        };
        "XF86AudioPause" = {
          action.spawn-sh = "playerctl pause --player=spotify";
          allow-when-locked = true;
        };
        "XF86AudioStop" = {
          action.spawn-sh = "playerctl pause --player=spotify";
          allow-when-locked = true;
        };
        "XF86AudioPrev" = {
          action.spawn-sh = "playerctl previous --player=spotify";
          allow-when-locked = true;
        };
        "XF86AudioNext" = {
          action.spawn-sh = "playerctl next --player=spotify";
          allow-when-locked = true;
        };

        "XF86MonBrightnessUp" = {
          action.spawn = [
            "brightnessctl"
            "--class=backlight"
            "set"
            "+10%"
          ];
          allow-when-locked = true;
        };
        "XF86MonBrightnessDown" = {
          action.spawn = [
            "brightnessctl"
            "--class=backlight"
            "set"
            "10%-"
          ];
          allow-when-locked = true;
        };

        "Mod+O" = {
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
        "Mod+Ctrl+H".action.move-column-left = [ ];
        "Mod+Ctrl+J".action.move-window-down = [ ];
        "Mod+Ctrl+K".action.move-window-up = [ ];
        "Mod+Ctrl+L".action.move-column-right = [ ];

        "Mod+Home".action.focus-column-first = [ ];
        "Mod+End".action.focus-column-last = [ ];
        "Mod+Ctrl+Home".action.move-column-to-first = [ ];
        "Mod+Ctrl+End".action.move-column-to-last = [ ];

        "Mod+Shift+Left".action.focus-monitor-left = [ ];
        "Mod+Shift+Down".action.focus-monitor-down = [ ];
        "Mod+Shift+Up".action.focus-monitor-up = [ ];
        "Mod+Shift+Right".action.focus-monitor-right = [ ];
        "Mod+Shift+H".action.focus-monitor-left = [ ];
        "Mod+Shift+J".action.focus-monitor-down = [ ];
        "Mod+Shift+K".action.focus-monitor-up = [ ];
        "Mod+Shift+L".action.focus-monitor-right = [ ];

        "Mod+Shift+Ctrl+Left".action.move-column-to-monitor-left = [ ];
        "Mod+Shift+Ctrl+Down".action.move-column-to-monitor-down = [ ];
        "Mod+Shift+Ctrl+Up".action.move-column-to-monitor-up = [ ];
        "Mod+Shift+Ctrl+Right".action.move-column-to-monitor-right = [ ];
        "Mod+Shift+Ctrl+H".action.move-column-to-monitor-left = [ ];
        "Mod+Shift+Ctrl+J".action.move-column-to-monitor-down = [ ];
        "Mod+Shift+Ctrl+K".action.move-column-to-monitor-up = [ ];
        "Mod+Shift+Ctrl+L".action.move-column-to-monitor-right = [ ];

        "Mod+Page_Down".action.focus-workspace-down = [ ];
        "Mod+Page_Up".action.focus-workspace-up = [ ];
        "Mod+U".action.focus-workspace-down = [ ];
        "Mod+I".action.focus-workspace-up = [ ];
        "Mod+Ctrl+Page_Down".action.move-column-to-workspace-down = [ ];
        "Mod+Ctrl+Page_Up".action.move-column-to-workspace-up = [ ];
        "Mod+Ctrl+U".action.move-column-to-workspace-down = [ ];
        "Mod+Ctrl+I".action.move-column-to-workspace-up = [ ];

        "Mod+Shift+Page_Down".action.move-workspace-down = [ ];
        "Mod+Shift+Page_Up".action.move-workspace-up = [ ];
        "Mod+Shift+U".action.move-workspace-down = [ ];
        "Mod+Shift+I".action.move-workspace-up = [ ];

        "Mod+WheelScrollDown" = {
          action.focus-workspace-down = [ ];
          cooldown-ms = 150;
        };
        "Mod+WheelScrollUp" = {
          action.focus-workspace-up = [ ];
          cooldown-ms = 150;
        };

        "Mod+Shift+WheelScrollDown".action.focus-column-right = [ ];
        "Mod+Shift+WheelScrollUp".action.focus-column-left = [ ];

        "Mod+1".action.focus-workspace = 1;
        "Mod+2".action.focus-workspace = 2;
        "Mod+3".action.focus-workspace = 3;
        "Mod+4".action.focus-workspace = 4;
        "Mod+5".action.focus-workspace = 5;
        "Mod+6".action.focus-workspace = 6;
        "Mod+7".action.focus-workspace = 7;
        "Mod+8".action.focus-workspace = 8;
        "Mod+9".action.focus-workspace = 9;
        "Mod+Ctrl+1".action.move-column-to-workspace = 1;
        "Mod+Ctrl+2".action.move-column-to-workspace = 2;
        "Mod+Ctrl+3".action.move-column-to-workspace = 3;
        "Mod+Ctrl+4".action.move-column-to-workspace = 4;
        "Mod+Ctrl+5".action.move-column-to-workspace = 5;
        "Mod+Ctrl+6".action.move-column-to-workspace = 6;
        "Mod+Ctrl+7".action.move-column-to-workspace = 7;
        "Mod+Ctrl+8".action.move-column-to-workspace = 8;
        "Mod+Ctrl+9".action.move-column-to-workspace = 9;

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

        "Print".action.screenshot = [ ];
        "Ctrl+Print".action.screenshot-screen = [ ];
        "Alt+Print".action.screenshot-window = [ ];

        "Mod+Escape" = {
          action.toggle-keyboard-shortcuts-inhibit = [ ];
          allow-inhibiting = false;
        };

        "Mod+Shift+E".action.quit = [ ];
        "Ctrl+Alt+Delete".action.quit = [ ];

        "Mod+Shift+P".action.power-off-monitors = [ ];
      };

      window-rules = [
        {
          matches = [
            { app-id = "^xdg-desktop-portal-gtk$"; }
            { app-id = "^nz\\.co\\.mega\\.$"; }
          ];
          open-floating = true;
        }

        {
          matches = [
            { app-id = "^firefox$"; }
            { app-id = "^dev\\.zed\\.Zed$"; }
            { app-id = "^spotify$"; }
          ];
          open-maximized = true;
        }

        {
          matches = [
            { app-id = "^org\\.telegram\\.desktop$"; }
            { app-id = "^org\\.keepassxc\\.KeePassXC$"; }
          ];
          open-maximized = true;
          block-out-from = "screen-capture";
        }

        {
          matches = [
            {
              app-id = "steam";
              title = "^notificationtoasts_\d+_desktop$";
            }
          ];
          block-out-from = "screen-capture";
          default-floating-position = {
            x = 10;
            y = 10;
            relative-to = "bottom-right";
          };
        }

        {
          default-column-width = {
            proportion = 0.5;
          };
          shadow = {
            enable = true;
            draw-behind-window = true;
          };
          geometry-corner-radius = {
            top-left = 6.0;
            top-right = 6.0;
            bottom-left = 6.0;
            bottom-right = 6.0;
          };
          clip-to-geometry = true;
        }
      ];

      layer-rules = [
        {
          matches = [ { namespace = "^notifications$"; } ];
          block-out-from = "screen-capture";
        }
        {
          matches = [ { namespace = "^wallpaper$"; } ];
          place-within-backdrop = true;
        }
      ];

      spawn-at-startup = [
        # {
        #   command = [
        #     (lib.getExe pkgs.something)
        #   ];
        # }
      ];

      hotkey-overlay.skip-at-startup = true;
      prefer-no-csd = true;
    };

    gtk = {
      enable = true;
      iconTheme = {
        package = pkgs.papirus-icon-theme;
        name = "Papirus-Dark";
      };
    };

    xdg.portal = {
      enable = true;
      xdgOpenUsePortal = true;

      extraPortals = with pkgs; [
        kdePackages.xdg-desktop-portal-kde
        xdg-desktop-portal-gtk
        xdg-desktop-portal-wlr
      ];

      config.common = {
        default = [
          "kde"
          "gtk"
        ];
        "org.freedesktop.impl.portal.ScreenCast" = "kde";
        "org.freedesktop.impl.portal.FileChooser" = "kde";
      };
    };

    systemd.user.services.swaybg = {
      Unit = {
        Description = "Wayland wallpaper daemon";
        ConditionEnvironment = "WAYLAND_DISPLAY";
        After = [ "niri.service" ];
        Requires = [ "niri.service" ];
        PartOf = [ "graphical-session.target" ];
      };
      Install = {
        WantedBy = [ "graphical-session.target" ];
      };
      Service = {
        ExecStart = "${lib.getExe pkgs.swaybg} -i ${pkgs.${namespace}.custom-wallpaper}";
        Restart = "on-failure";
      };
    };
  };
}

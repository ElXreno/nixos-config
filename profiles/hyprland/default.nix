{
  config,
  pkgs,
  lib,
  ...
}:
let
  defaultWallpaper = (import ./wallpapers { inherit pkgs; }).default;

  update-mic-state = pkgs.writeScript "update-mic-state" ''
    #!${pkgs.bash}/bin/bash
    IS_MUTED=0
    if ${pkgs.wireplumber}/bin/wpctl get-volume @DEFAULT_AUDIO_SOURCE@ | ${pkgs.gnugrep}/bin/grep -q "MUTED"; then
      IS_MUTED=1
    fi

    echo $IS_MUTED > /sys/class/leds/platform::micmute/brightness
  '';
in
{
  imports = [
    ./hypridle.nix
    ./hyprlock.nix
    ./waybar.nix
  ];

  services.displayManager.sddm = {
    enable = true;
    wayland.enable = true;
    package = pkgs.kdePackages.sddm;

    theme = "sddm-astronaut-theme";

    extraPackages = with pkgs; [
      kdePackages.qtmultimedia
      kdePackages.qtsvg
      kdePackages.qtvirtualkeyboard
    ];
  };

  environment.systemPackages =
    let
      sddm-astronaut = pkgs.sddm-astronaut.override {
        embeddedTheme = "pixel_sakura_static";
        themeConfig = {
          Background = toString defaultWallpaper;

          FormPosition = "left";

          BackgroundColor = "#141821";
          DimBackgroundColor = "#10131A";
          FormBackgroundColor = "#1B1E25";

          DateTextColor = "#93A4B5";
          TimeTextColor = "#E6EDF3";

          PlaceholderTextColor = "#A9B4BF";

          LoginFieldBackgroundColor = "#d4d9e9ff";
          PasswordFieldBackgroundColor = "#d4d9e9ff";
          LoginFieldTextColor = "#E2B714";
          PasswordFieldTextColor = "#E2B714";

          UserIconColor = "#E2B714";
          PasswordIconColor = "#E2B714";

          SessionButtonTextColor = "#93A4B5";

          HighlightBackgroundColor = "#E2B714";
          HighlightTextColor = "#0E1117";
          HighlightBorderColor = "transparent";

          HoverSessionButtonTextColor = "#F3CC26";

          WarningColor = "#D6A50F";
        };
      };
    in
    [
      sddm-astronaut
    ];

  services.power-profiles-daemon.enable = true;

  programs.hyprland = {
    enable = true;
    withUWSM = true;
  };

  xdg.portal = {
    enable = true;
    extraPortals = with pkgs; [
      xdg-desktop-portal-hyprland
      xdg-desktop-portal-gtk
      kdePackages.xdg-desktop-portal-kde
    ];

    xdgOpenUsePortal = true;
    config.common.default = "kde";
  };

  environment.sessionVariables = {
    NIXOS_OZONE_WL = "1";
    GDK_BACKEND = "wayland,x11";
    QT_QPA_PLATFORM = "wayland;xcb";
    QT_WAYLAND_DISABLE_WINDOWDECORATION = 1;
  };

  services.udev.extraRules = ''
    SUBSYSTEM=="leds", KERNEL=="platform::micmute", RUN="${pkgs.coreutils}/bin/chmod a+rw /sys/class/leds/platform::micmute/brightness"
  '';

  home-manager.users.elxreno = {
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

      plugins = with pkgs; [
        split-monitor-workspaces
      ];

      settings = {
        "$mod" = "SUPER";
        "$terminal" = "${pkgs.kitty}/bin/kitty";
        "$menu" = "wofi -S drun";
        "$fileManager" = "thunar";

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

        gesture = [ "3, horizontal, workspace" ];

        monitor = [
          "eDP-1, 1920x1080@144, 0x0, 1"
          "eDP-2, 1920x1080@144, 0x0, 1"
          "DP-2, preferred, -1920x0, 1.0"
          "HDMI-A-1, 1920x1080@60, -1920x0, 1.0"
        ];

        cursor = {
          default_monitor = "eDP-1";
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
          "$mod, R, exec, $menu"
          "$mod, E, exec, $fileManager"
          "$mod, H, exec, cliphist list | wofi -d | cliphist decode | wl-copy"
          "$mod, L, exec, hyprlock"

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
          ", XF86MonBrightnessUp,   exec, brightnessctl set +5%"
          ", XF86MonBrightnessDown, exec, brightnessctl set 5%-"

          ", XF86AudioRaiseVolume, exec, wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%+ -l 1"
          ", XF86AudioLowerVolume, exec, wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%- -l 1"
        ];

        bindl = [
          ", XF86AudioMute,    exec, wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"
          ", XF86AudioMicMute, exec, wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle${
            lib.optionalString (config.device == "INFINITY") " && ${update-mic-state}"
          }"
          ", XF86AudioPlay, exec, playerctl play --player=spotify"
          ", XF86AudioPause, exec, playerctl pause --player=spotify"
          ", XF86AudioPrev, exec, playerctl previous --player=spotify"
          ", XF86AudioNext, exec, playerctl next --player=spotify"

          # " , switch:Lid Switch, exec, hyprlock" # Remove it if I will use suspend mode
          " , switch:on:Lid Switch, exec, hyprctl keyword monitor 'eDP-1, disable'"
          " , switch:off:Lid Switch, exec, hyprctl keyword monitor 'eDP-1, preffered, auto, 1'"
        ];

        windowrulev2 = [
          "float, class:^(MEGAsync)$"
          "noborder, class:^(MEGAsync)$"

          "float, class:^(steam)$, title:^(Friends List)$"
          "float, class:^(org.freedesktop.impl.portal.desktop.kde)$"

          "opacity 0.0 override 0.0 override, class:^(xwaylandvideobridge)$"
          "noanim, class:^(xwaylandvideobridge)$"
          "nofocus, class:^(xwaylandvideobridge)$"
          "noinitialfocus, class:^(xwaylandvideobridge)$"

          "workspace 1, class:^(firefox)$"
          "workspace 5, class:^(org.telegram.desktop)$"
          "workspace 9, class:^(spotify)$"
          "workspace 10, class:^(org.keepassxc.KeePassXC)$"

          "immediate, class:^(DDNet)$"
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
          (lib.mkIf (config.device == "INFINITY") "sleep 3 && ${update-mic-state}")
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

    services.hyprpaper = {
      enable = true;
      settings = {
        preload = [
          (toString defaultWallpaper)
        ];

        wallpaper = ", ${toString defaultWallpaper}";
      };
    };
  };

  fonts.packages = with pkgs; [
    jetbrains-mono
    nerd-fonts.jetbrains-mono
  ];
}

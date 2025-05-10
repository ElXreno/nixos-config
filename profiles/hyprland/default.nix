{ pkgs, inputs, ... }:
let
  update-mic-state = pkgs.writeScript "update-mic-state" ''
    #!${pkgs.bash}/bin/bash
    IS_MUTED=0
    if ${pkgs.wireplumber}/bin/wpctl get-volume @DEFAULT_AUDIO_SOURCE@ | ${pkgs.gnugrep}/bin/grep -q "MUTED"; then
      IS_MUTED=1
    fi

    echo $IS_MUTED > /sys/class/leds/platform::micmute/brightness
  '';
  hypr_prefix = inputs.hyprland.packages.${pkgs.stdenv.hostPlatform.system};
  hyprland = hypr_prefix.hyprland;
  xdg-desktop-portal-hyprland = hypr_prefix.xdg-desktop-portal-hyprland;
in {
  imports = [ ./hypridle.nix ./hyprlock.nix ./waybar.nix ];

  services.displayManager.sddm.enable = true;
  services.displayManager.sddm.wayland.enable = true;

  services.power-profiles-daemon.enable = true;

  programs.hyprland = {
    enable = true;

    package = hyprland;
    portalPackage = xdg-desktop-portal-hyprland;
  };

  # Optional, hint Electron apps to use Wayland:
  environment.sessionVariables.NIXOS_OZONE_WL = "1";

  services.udev.extraRules = ''
    SUBSYSTEM=="leds", KERNEL=="platform::micmute", RUN="${pkgs.coreutils}/bin/chmod a+rw /sys/class/leds/platform::micmute/brightness"
  '';

  home-manager.users.elxreno = {
    services.hyprpolkitagent.enable = true;

    xdg.portal.extraPortals = with pkgs; [
      xdg-desktop-portal-gtk
      kdePackages.xdg-desktop-portal-kde
    ];

    home.packages = with pkgs; [
      xfce.thunar
      xfce.tumbler
      qimgv
      waybar
      pavucontrol
      networkmanagerapplet
      dunst
      blueman
      wl-clipboard
      cliphist
      wofi
      brightnessctl
      hyprcursor
    ];

    home.pointerCursor = {
      name = "Breeze_Hacked";
      size = 24;
      package = pkgs.breeze-hacked-cursor-theme;

      enable = true;

      x11.enable = true;
      gtk.enable = true;
      hyprcursor = { enable = true; };
    };
    programs.kitty.enable = true; # required for the default Hyprland config
    wayland.windowManager.hyprland = {
      enable = true;

      package = null;
      portalPackage = null;

      settings = {
        "$mod" = "SUPER";
        "$terminal" = "${pkgs.kitty}/bin/kitty";
        "$menu" = "wofi -S drun";
        "$fileManager" = "thunar";

        input = {
          kb_layout = "us,ru";
          kb_options = "grp:alt_shift_toggle";

          accel_profile = "flat";

          touchpad = {
            natural_scroll = true;
            middle_button_emulation = true;
          };
        };

        gestures = {
          workspace_swipe = true;
          workspace_swipe_fingers = 3;
          workspace_swipe_distance = 300;
          workspace_swipe_invert = true;
          workspace_swipe_min_speed_to_force = 30;
          workspace_swipe_forever = true;
        };

        monitor = [
          "eDP-1, preffered, auto, 1"
          "HDMI-A-1, preffered, -1920x0, 1.5, vrr, 1"
        ];

        decoration = {
          rounding = 10;
          shadow = { enabled = true; };

          active_opacity = 1.0;
          inactive_opacity = 0.93;
          fullscreen_opacity = 1.0;
        };

        bind = [
          ", Print, exec, ${pkgs.hyprshot}/bin/hyprshot -m region --clipboard-only"
          "$mod, B, exec, firefox"
          "$mod, Return, exec, $terminal" # Are you fucking crazy? Why return?
          "$mod, R, exec, $menu"
          "$mod, E, exec, $fileManager"
          "$mod, H, exec, cliphist list | wofi -d | cliphist decode | wl-copy"
          "$mod, L, exec, hyprlock"

          "$mod, M, exit,"
          "$mod, C, killactive,"
          "$mod, J, togglesplit,"

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
        ] ++ (
          # workspaces
          # binds $mod + [shift +] {1..9} to [move to] workspace {1..9}
          builtins.concatLists (builtins.genList (i:
            let ws = i + 1;
            in [
              "$mod, code:1${toString i}, workspace, ${toString ws}"
              "$mod SHIFT, code:1${toString i}, movetoworkspace, ${toString ws}"
            ]) 9));

        bindle = [
          ", XF86MonBrightnessUp,   exec, brightnessctl -d amdgpu_bl1 set +5%"
          ", XF86MonBrightnessDown, exec, brightnessctl -d amdgpu_bl1 set 5%-"

          ", XF86AudioRaiseVolume, exec, wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%+"
          ", XF86AudioLowerVolume, exec, wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%-"
        ];

        bindl = [
          ", XF86AudioMute,    exec, wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"
          ", XF86AudioMicMute, exec, wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle && ${update-mic-state}"

          # " , switch:Lid Switch, exec, hyprlock" # Remove it if I will use suspend mode
          " , switch:on:Lid Switch, exec, hyprctl keyword monitor 'eDP-1, disable'"
          " , switch:off:Lid Switch, exec, hyprctl keyword monitor 'eDP-1, preffered, auto, 1'"
        ];

        exec-once = [
          "waybar"
          "nm-applet --indicator"
          "dunst"
          "blueman-applet"
          "wl-paste --type text --watch cliphist store"
          "wl-paste --type image --watch cliphist store"
          "sleep 3 && ${update-mic-state}"
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

{ pkgs, ... }: {
  imports = [ ./waybar.nix ];

  services.displayManager.sddm.enable = true;
  services.displayManager.sddm.wayland.enable = true;

  services.power-profiles-daemon.enable = true;

  programs.hyprland.enable = true; # enable Hyprland

  # Optional, hint Electron apps to use Wayland:
  environment.sessionVariables.NIXOS_OZONE_WL = "1";

  home-manager.users.elxreno = {
    xdg.portal.extraPortals = with pkgs; [
      xdg-desktop-portal-gtk
      kdePackages.xdg-desktop-portal-kde
    ];

    home.packages = with pkgs; [ waybar pavucontrol ];
    programs.kitty.enable = true; # required for the default Hyprland config
    wayland.windowManager.hyprland = {
      enable = true;

      settings = {
        "$mod" = "SUPER";
        "$terminal" = "${pkgs.kitty}/bin/kitty";
        "$menu" = "${pkgs.wofi}/bin/wofi --show drun";

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

        monitor = [ "eDP-1, preffered, auto, 1" ];

        decoration = {
          rounding = 10;
          shadow = { enabled = true; };

          active_opacity = 1.0;
          inactive_opacity = 0.93;
          fullscreen_opacity = 1.0;
        };

        bind = [
          ", Print, exec, ${pkgs.grimblast}/bin/grimblast copy area"
          "$mod, F, exec, firefox"
          "$mod, Return, exec, $terminal" # Are you fucking crazy? Why return?
          "$mod, R, exec, $menu"
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

        exec-once = [ "waybar" ];
      };
    };

    gtk = {
      enable = true;

      theme = {
        package = pkgs.flat-remix-gtk;
        name = "Flat-Remix-GTK-Grey-Darkest";
      };

      iconTheme = {
        package = pkgs.adwaita-icon-theme;
        name = "Adwaita";
      };

      font = {
        name = "Sans";
        size = 11;
      };
    };
  };
}

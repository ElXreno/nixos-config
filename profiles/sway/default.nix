{ config, pkgs, lib, ... }:

{
  programs.sway = {
    enable = true;
    wrapperFeatures.gtk = true;
  };

  environment.loginShellInit = lib.mkAfter ''
    [[ "$(tty)" == /dev/tty1 ]] && {
      sway
    }
  '';

  environment.sessionVariables = {
    _JAVA_AWT_WM_NONREPARENTING = "1";
    MOZ_ENABLE_WAYLAND = "1";
    QT_QPA_PLATFORM = "wayland";
    QT_WAYLAND_DISABLE_WINDOWDECORATION = "1";
  };

  fonts.fonts = with pkgs; [
    font-awesome
    (nerdfonts.override { fonts = [ "FiraCode" "JetBrainsMono" "Hermit" ]; })
  ];

  fonts.enableDefaultFonts = true;

  home-manager.users.elxreno = {
    imports = [
      ./rofi.nix
      ./sway-bar.nix
      ./sway-notifications.nix
      ./swayidle.nix
      ./swaylock.nix
    ];
    home.packages = with pkgs; [ ranger ];
    wayland.windowManager.sway = let
      rofiPackage =
        config.home-manager.users.elxreno.programs.rofi.finalPackage;
    in {
      enable = true;
      config = rec {
        focus = {
          followMouse = false;
          wrapping = "force";
        };

        window = {
          border = 1;
          titlebar = false;
          commands = [
            {
              criteria = { window_role = "pop-up"; };
              command = "no_focus";
            }
            {
              criteria = { window_type = "notification"; };
              command = "no_focus";
            }
            {
              criteria = { title = "KeePassXC - Browser Access Request"; };
              command = "floating enable";
            }
          ];
        };

        fonts = {
          names = [ "FiraCode Nerd Font" ];
          size = 9.0;
        };

        modifier = "Mod4";

        startup = map (command: { inherit command; })
          [ "${pkgs.megasync}/bin/megasync" ];

        keybindings = let
          workspaces = lib.lists.drop 1
            (builtins.genList (x: [ (toString x) (toString x) ]) 10);
        in {
          "${modifier}+F5" = "reload";
          "${modifier}+Shift+q" = "kill";
          "${modifier}+Shift+e" =
            "exec swaynag -t warning -m 'Do you want to exit sway?' -b 'Yes' 'swaymsg exit'";

          "${modifier}+Return" = "exec ${pkgs.alacritty}/bin/alacritty";
          "${modifier}+d" = "exec ${rofiPackage}/bin/rofi -show drun";
          "${modifier}+c" = ''
            exec ${rofiPackage}/bin/rofi -show calc -modi calc -no-show-match -no-sort -automatic-save-to-history -calc-command "echo -n '{result}' | wl-copy"'';
          "${modifier}+l" = "exec loginctl lock-session";
          "Print" =
            "exec ${pkgs.sway-contrib.grimshot}/bin/grimshot copy screen";
          "${modifier}+Print" =
            "exec ${pkgs.sway-contrib.grimshot}/bin/grimshot copy area";

          "${modifier}+Up" = "focus child; focus up";
          "${modifier}+Down" = "focus child; focus down";
          "${modifier}+Right" = "focus child; focus right";
          "${modifier}+Left" = "focus child; focus left";
          "${modifier}+Shift+Up" = "move up";
          "${modifier}+Shift+Down" = "move down";
          "${modifier}+Shift+Right" = "move right";
          "${modifier}+Shift+Left" = "move left";

          "${modifier}+Shift+h" = "layout splith";
          "${modifier}+Shift+v" = "layout splitv";
          "${modifier}+h" = "split h";
          "${modifier}+v" = "split v";

          "${modifier}+r" = ''mode "resize"'';

          "XF86MonBrightnessDown" =
            "exec ${pkgs.brightnessctl}/bin/brightnessctl s 5%-";
          "XF86MonBrightnessUp" =
            "exec ${pkgs.brightnessctl}/bin/brightnessctl s 5%+";

          "XF86AudioLowerVolume" = "exec ${pkgs.pamixer}/bin/pamixer -d 5";
          "XF86AudioRaiseVolume" = "exec ${pkgs.pamixer}/bin/pamixer -i 5";
          "XF86AudioMute" = "exec ${pkgs.pamixer}/bin/pamixer -t";
          "XF86AudioMicMute" =
            "exec ${pkgs.pamixer}/bin/pamixer --default-source -t";

          "XF86AudioPlay" = "exec ${pkgs.playerctl}/bin/playerctl play-pause";
          "XF86AudioPause" = "exec ${pkgs.playerctl}/bin/playerctl play-pause";
          "XF86AudioNext" = "exec ${pkgs.playerctl}/bin/playerctl next";
          "XF86AudioPrev" = "exec ${pkgs.playerctl}/bin/playerctl previous";

          "${modifier}+Comma" = "workspace prev";
          "${modifier}+Period" = "workspace next";
        } // builtins.listToAttrs (builtins.map (x: {
          name = "${modifier}+${builtins.elemAt x 0}";
          value = "workspace ${builtins.elemAt x 1}";
        }) workspaces) // builtins.listToAttrs (builtins.map (x: {
          name = "${modifier}+Shift+${builtins.elemAt x 0}";
          value = "move container to workspace ${builtins.elemAt x 1}";
        }) workspaces);

        modes.resize = {
          "Left" = "resize shrink width 10px";
          "Down" = "resize grow height 10px";
          "Up" = "resize shrink height 10px";
          "Right" = "resize grow width 10px";

          "Return" = ''mode "default"'';
          "Escape" = ''mode "default"'';
        };

        input = {
          "*" = {
            xkb_layout = "us,ru";
            xkb_options = "grp:alt_shift_toggle";
          };
          "type:pointer" = { accel_profile = "flat"; };
          "type:touchpad" = {
            tap = "enabled";
            scroll_factor = "0.45";
            natural_scroll = "enabled";
            dwt = "enabled";
          };
        };

        output = {
          "eDP-1" = {
            bg =
              "/home/elxreno/Pictures/Wallpapers/wolf_silhouette_moon_night_118727_1920x1080.jpg fill";
          };
        };
      };
    };
  };

  services.dbus.enable = true;
  xdg.portal = {
    enable = true;
    wlr.enable = true;
    extraPortals = [ pkgs.xdg-desktop-portal-gtk ];
  };
}

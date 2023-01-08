{ config, pkgs, lib, ... }:

let
  lock = pkgs.writeScript "lock" ''
    ${pkgs.swaylock-effects}/bin/swaylock -f --screenshots --clock --effect-greyscale
  '';
  unlock = pkgs.writeScript "unlock" ''
    ${pkgs.procps}/bin/pkill swaylock
  '';
  screen-off = pkgs.writeScript "screenOff" ''
    ${pkgs.sway}/bin/swaymsg "output * dpms off"
  '';
  resume = pkgs.writeScript "resume" ''
    ${pkgs.sway}/bin/swaymsg "output * dpms on"
  '';
in
{
  imports = [ ./rofi.nix (import ./swayidle.nix { inherit pkgs lock unlock screen-off resume; }) ./swaylock.nix ./waybar.nix ];

  programs.sway = {
    enable = true;
    wrapperFeatures.gtk = true;
    extraPackages = with pkgs; [ swayidle xwayland foot ];
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
    wayland.windowManager.sway = {
      enable = true;
      config = rec {
        focus = {
          followMouse = false;
          forceWrapping = true;
        };
        modifier = "Mod4";
        window = {
          border = 0;
          titlebar = false;
        };
        bars = [
          {
            command = "${pkgs.waybar}/bin/waybar";
          }
        ];
        startup = map (command: { inherit command; }) [ "${pkgs.megasync}/bin/megasync" ]
          #   ++ [
          #   {
          #     command =
          #       "swayidle -w before-sleep '${lock}' lock '${lock}' unlock '${unlock}' timeout 600 '${screen-off}' resume '${resume}'";
          #   }
          # ]
        ;
        keybindings =
          let
            workspaces = (lib.lists.drop 1 (builtins.genList (x: [ (toString x) (toString x) ]) 10));
            # workspaces = builtins.genList (x: [ (toString x) (toString (if x == 0 then 10 else x)) ]) 10;
          in
          {
            "${modifier}+F5" = "reload";

            "${modifier}+Return" = "exec ${pkgs.alacritty}/bin/alacritty";
            "${modifier}+d" = "exec ${pkgs.rofi}/bin/rofi -show drun";
            "${modifier}+l" = "exec ${lock}";
            "Print" = "exec ${pkgs.sway-contrib.grimshot}/bin/grimshot copy screen";
            "${modifier}+Print" = "exec ${pkgs.sway-contrib.grimshot}/bin/grimshot copy area";

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

            "XF86MonBrightnessDown" = "exec ${pkgs.brightnessctl}/bin/brightnessctl s 5%-";
            "XF86MonBrightnessUp" = "exec ${pkgs.brightnessctl}/bin/brightnessctl s 5%+";

            "XF86AudioLowerVolume" = "exec ${pkgs.pamixer}/bin/pamixer -d 5";
            "XF86AudioRaiseVolume" = "exec ${pkgs.pamixer}/bin/pamixer -i 5";
            "XF86AudioMute" = "exec ${pkgs.pamixer}/bin/pamixer -t";
            "XF86AudioMicMute" = "exec ${pkgs.pamixer}/bin/pamixer --default-source -t";

            "XF86AudioPlay" = "exec ${pkgs.playerctl}/bin/playerctl play-pause";
            "XF86AudioPause" = "exec ${pkgs.playerctl}/bin/playerctl play-pause";
            "XF86AudioNext" = "exec ${pkgs.playerctl}/bin/playerctl next";
            "XF86AudioPrev" = "exec ${pkgs.playerctl}/bin/playerctl previous";

            "${modifier}+Comma" = "workspace prev";
            "${modifier}+Period" = "workspace next";
          } // builtins.listToAttrs (builtins.map
            (x: {
              name = "${modifier}+${builtins.elemAt x 0}";
              value = "workspace ${builtins.elemAt x 1}";
            })
            workspaces) // builtins.listToAttrs (builtins.map
            (x: {
              name = "${modifier}+Shift+${builtins.elemAt x 0}";
              value = "move container to workspace ${builtins.elemAt x 1}";
            })
            workspaces);
        input = {
          "*" = {
            xkb_layout = "us,ru";
            xkb_options = "grp:alt_shift_toggle";
          };
          "10182:480:GXTP7863:00_27C6:01E0_Touchpad" = {
            tap = "enabled";
            natural_scroll = "enabled";
            dwt = "enabled";
          };
        };
        output = {
          "eDP-1" = {
            bg = "/home/elxreno/Pictures/Wallpapers/wolf_silhouette_moon_night_118727_1920x1080.jpg fill";
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

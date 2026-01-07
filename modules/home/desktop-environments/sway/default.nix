{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.sway;
in
{
  options.${namespace}.desktop-environments.sway = {
    enable = mkEnableOption "Whether or not to manage sway.";
  };

  config = mkIf cfg.enable {
    ${namespace}.programs = {
      waybar.enable = true;
    };

    programs.kitty.enable = true;
    services.autotiling.enable = true;

    home.packages = with pkgs; [
      grim
      slurp
      wl-clipboard
      mako
      networkmanagerapplet
      blueman
      swaysome
    ];

    # fonts.fontconfig = {
    #   enable = true;
    #   antialiasing = true;
    #   hinting = "slight";
    #   subpixelRendering = "rgb";
    # };

    wayland.windowManager.sway = {
      enable = true;
      wrapperFeatures.gtk = true;
      systemd.enable = true;
      extraOptions = [ "--unsupported-gpu" ];

      config = rec {
        modifier = "Mod4";
        terminal = "kitty";

        input = {
          "type:keyboard" = {
            xkb_layout = "us,ru";
            xkb_options = "grp:win_space_toggle";
          };
        };

        bars = [ ];

        gaps = {
          inner = 5;
          outer = 5;
        };

        startup = [
          { command = "swaysome init 1"; }
          { command = "nm-applet --indicator"; }
          { command = "blueman-applet"; }
        ];

        keybindings = lib.mkOptionDefault {
          # Change focus between workspaces
          "${modifier}+1" = "exec swaysome focus 1";
          "${modifier}+2" = "exec swaysome focus 2";
          "${modifier}+3" = "exec swaysome focus 3";
          "${modifier}+4" = "exec swaysome focus 4";
          "${modifier}+5" = "exec swaysome focus 5";
          "${modifier}+6" = "exec swaysome focus 6";
          "${modifier}+7" = "exec swaysome focus 7";
          "${modifier}+8" = "exec swaysome focus 8";
          "${modifier}+9" = "exec swaysome focus 9";
          "${modifier}+0" = "exec swaysome focus 0";

          # Move containers between workspaces
          "${modifier}+Shift+1" = "exec swaysome move 1";
          "${modifier}+Shift+2" = "exec swaysome move 2";
          "${modifier}+Shift+3" = "exec swaysome move 3";
          "${modifier}+Shift+4" = "exec swaysome move 4";
          "${modifier}+Shift+5" = "exec swaysome move 5";
          "${modifier}+Shift+6" = "exec swaysome move 6";
          "${modifier}+Shift+7" = "exec swaysome move 7";
          "${modifier}+Shift+8" = "exec swaysome move 8";
          "${modifier}+Shift+9" = "exec swaysome move 9";
          "${modifier}+Shift+0" = "exec swaysome move 0";

          # Focus workspace groups
          "${modifier}+Alt+1" = "exec swaysome focus-group 1";
          "${modifier}+Alt+2" = "exec swaysome focus-group 2";
          "${modifier}+Alt+3" = "exec swaysome focus-group 3";
          "${modifier}+Alt+4" = "exec swaysome focus-group 4";
          "${modifier}+Alt+5" = "exec swaysome focus-group 5";
          "${modifier}+Alt+6" = "exec swaysome focus-group 6";
          "${modifier}+Alt+7" = "exec swaysome focus-group 7";
          "${modifier}+Alt+8" = "exec swaysome focus-group 8";
          "${modifier}+Alt+9" = "exec swaysome focus-group 9";
          "${modifier}+Alt+0" = "exec swaysome focus-group 0";

          # Move containers to other workspace groups
          "${modifier}+Alt+Shift+1" = "exec swaysome move-to-group 1";
          "${modifier}+Alt+Shift+2" = "exec swaysome move-to-group 2";
          "${modifier}+Alt+Shift+3" = "exec swaysome move-to-group 3";
          "${modifier}+Alt+Shift+4" = "exec swaysome move-to-group 4";
          "${modifier}+Alt+Shift+5" = "exec swaysome move-to-group 5";
          "${modifier}+Alt+Shift+6" = "exec swaysome move-to-group 6";
          "${modifier}+Alt+Shift+7" = "exec swaysome move-to-group 7";
          "${modifier}+Alt+Shift+8" = "exec swaysome move-to-group 8";
          "${modifier}+Alt+Shift+9" = "exec swaysome move-to-group 9";
          "${modifier}+Alt+Shift+0" = "exec swaysome move-to-group 0";

          # Move focused container to next output
          "${modifier}+o" = "exec swaysome next-output";
          # Move focused container to previous output
          "${modifier}+Shift+o" = "exec swaysome prev-output";

          # Move focused workspace group to next output
          "${modifier}+Alt+o" = "exec swaysome workspace-group-next-output";
          # Move focused workspace group to previous output
          "${modifier}+Alt+Shift+o" = "exec swaysome workspace-group-prev-output";
        };
      };
    };

    services.kanshi = {
      enable = true;
      systemdTarget = "sway-session.target";

      settings = [
        {
          profile.name = "undocked";
          profile.outputs = [
            {
              criteria = "eDP-1";
              status = "enable";
              mode = "1920x1080@144.063Hz";
              position = "0,0";
              scale = 1.0;
            }
          ];
        }

        {
          profile.name = "home-docked";
          profile.outputs = [
            {
              criteria = "Xiaomi Corporation Mi monitor 5392700044842";
              status = "enable";
              mode = "2560x1440@180.000Hz";
              position = "0,0";
            }
            {
              criteria = "eDP-1";
              status = "enable";
              mode = "1920x1080@144.063Hz";
              position = "-1920,180";
            }
          ];
        }

        {
          profile.name = "home-clamshell";
          profile.outputs = [
            {
              criteria = "eDP-1";
              status = "disable";
            }
            {
              criteria = "Xiaomi Corporation Mi monitor 5392700044842";
              status = "enable";
              mode = "2560x1440@180.000Hz";
              position = "0,0";
            }
          ];
        }
      ];
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

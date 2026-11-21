{
  config,
  osConfig,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.niri;

  inherit (config.lib.stylix.colors.withHashtag)
    base00
    base04
    base03
    base08
    base0D
    ;
in
{
  options.${namespace}.desktop-environments.niri = {
    enable = mkEnableOption "managing niri" // {
      default = osConfig.${namespace}.desktop-environments.niri.enable;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      stylix.enable = true;

      programs = {
        kitty.enable = true;
        noctalia.enable = true;
      };
    };

    systemd.user.services.wayland-wm-app-daemon = {
      Unit = {
        Description = "Fast application argument generator (uwsm)";
        PartOf = "graphical-session.target";
        After = "graphical-session.target";
      };
      Service = {
        Type = "exec";
        ExecStart = "${pkgs.uwsm}/bin/uwsm aux app-daemon";
        Restart = "on-failure";
        Slice = "session.slice";
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };

    home = {
      packages = with pkgs; [
        xwayland-satellite
        playerctl
        pavucontrol
        qimgv
        thunar
        tumbler
        cliphist
        wl-clipboard
        kdePackages.ark
      ];

      pointerCursor = {
        enable = true;

        name = "Breeze_Hacked";
        size = 24;
        package = pkgs.breeze-hacked-cursor-theme;

        x11.enable = true;
        gtk.enable = true;
      };

      sessionVariables = {
        _JAVA_AWT_WM_NONREPARENTING = 1;

        QT_QPA_PLATFORM = "wayland;xcb";
        CLUTTER_BACKEND = "wayland";
        SDL_VIDEODRIVER = "wayland,x11";

        NIXOS_OZONE_WL = 1;
        ELECTRON_OZONE_PLATFORM_HINT = "wayland"; # Fallback for NIXOS_OZONE_WL, non-nix packaged software
      };
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

        touchpad = {
          tap = true;
          natural-scroll = true;
        };

        warp-mouse-to-focus.enable = true;

        focus-follows-mouse = {
          enable = true;
          max-scroll-amount = "0%";
        };
      };

      debug.enable-overlay-planes = mkIf osConfig.${namespace}.facts.gpu.amd.exists { };

      clipboard.disable-primary = true;

      layout = {
        gaps = 12;
        background-color = "transparent";
        border = {
          width = 2;
          active.gradient = {
            from = base03;
            to = base04;
            angle = 45;
            relative-to = "workspace-view";
          };
        };

        tab-indicator = {
          place-within-column = true;
          position = "top";
          width = 4;
          gap = 4;
          corner-radius = 2.0;
          active.color = base0D;
          inactive.color = base03;
          urgent.color = base08;
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

      overview = {
        backdrop-color = base00;
        workspace-shadow.enable = false;
      };

      cursor.hide-when-typing = true;

      hotkey-overlay.skip-at-startup = true;
      prefer-no-csd = true;
    };

    gtk.enable = true;

    xfconf.settings.thunar = {
      hidden-bookmarks = [ "file://${config.home.homeDirectory}/Desktop" ];
    };

    xdg.portal = {
      enable = true;
      xdgOpenUsePortal = true;

      extraPortals = with pkgs; [
        xdg-desktop-portal-gnome
        xdg-desktop-portal-gtk
      ];

      config = {
        common = {
          default = [
            "gtk"
          ];
          "org.freedesktop.impl.portal.ScreenCast" = [ "gnome" ];
          "org.freedesktop.impl.portal.Screenshot" = [ "gnome" ];
        };
      };
    };

    xdg.mimeApps.defaultApplications = {
      "inode/directory" = "thunar.desktop";

      "image/jpeg" = "qimgv.desktop";
      "image/png" = "qimgv.desktop";
      "image/gif" = "qimgv.desktop";
      "image/bmp" = "qimgv.desktop";
      "image/webp" = "qimgv.desktop";
      "image/tiff" = "qimgv.desktop";
      "image/avif" = "qimgv.desktop";
      "image/heif" = "qimgv.desktop";
      "image/heic" = "qimgv.desktop";
      "image/x-icon" = "qimgv.desktop";
      "image/vnd.microsoft.icon" = "qimgv.desktop";
      "image/x-tga" = "qimgv.desktop";
      "image/jp2" = "qimgv.desktop";
    };
  };
}

{
  config,
  namespace,
  lib,
  ...
}:
let
  cfg = config.${namespace}.desktop-environments.niri;
in
{
  config = lib.mkIf cfg.enable {
    programs.niri.settings.window-rules = [
      {
        default-column-width = {
          proportion = 0.5;
        };
        shadow = {
          enable = true;
          draw-behind-window = true;
        };
        geometry-corner-radius = {
          top-left = 8.0;
          top-right = 8.0;
          bottom-left = 8.0;
          bottom-right = 8.0;
        };
        clip-to-geometry = true;
      }

      {
        matches = [
          { app-id = "^xdg-desktop-portal-gtk$"; }
          { app-id = "^nz\\.co\\.mega\\.$"; }
        ];
        open-floating = true;
      }

      {
        matches = [
          { app-id = "^dev\\.zed\\.Zed$"; }
          { app-id = "^firefox$"; }
          { app-id = "^Freelens$"; }
          { app-id = "^spotify$"; }
          { app-id = "^steam$"; }
        ];
        excludes = [
          {
            app-id = "^steam$";
            title = "^Friends List$";
          }
        ];
        open-maximized = true;
      }

      {
        matches = [
          { app-id = "^org\\.telegram\\.desktop$"; }
        ];
        excludes = [
          {
            app-id = "^org\\.telegram\\.desktop$";
            title = "^Media viewer$";
          }
        ];
        open-maximized = true;
        block-out-from = "screencast";
      }
      {
        matches = [
          {
            app-id = "^org\\.telegram\\.desktop$";
            title = "^Media viewer$";
          }
        ];
        open-fullscreen = false;
        open-floating = true;
        default-column-width.proportion = 0.7;
        default-window-height.proportion = 0.7;
        block-out-from = "screencast";
      }

      {
        matches = [
          { app-id = "^org\\.keepassxc\\.KeePassXC$"; }
        ];
        open-maximized = true;
        open-focused = true;
        block-out-from = "screencast";
      }

      {
        matches = [
          {
            app-id = "^steam$";
            title = "^Friends List$";
          }
        ];
        open-floating = true;
        default-column-width.proportion = 0.3;
        default-window-height.proportion = 0.7;
      }
      {
        matches = [
          {
            app-id = "steam";
            title = "^notificationtoasts_\\d+_desktop$";
          }
        ];
        open-focused = false;
        default-floating-position = {
          x = 10;
          y = 10;
          relative-to = "bottom-right";
        };
        block-out-from = "screencast";
      }
    ];
  };
}

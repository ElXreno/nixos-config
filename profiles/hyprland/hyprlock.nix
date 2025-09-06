{ lib, pkgs, ... }:
let
  defaultWallpaper = (import ./wallpapers { inherit pkgs; }).default;
in
{
  home-manager.users.elxreno = {
    programs.hyprlock = {
      enable = true;

      settings = {
        general = {
          hide_cursor = true;
          ignore_empty_input = true;
        };

        background = [
          {
            path = toString defaultWallpaper;
            blur_passes = 0;
            noise = 0;
          }
        ];

        label =
          let
            playerctlMetadata = pkgs.writeShellScript "player-metadata" ''
              status=$(${lib.getExe pkgs.playerctl} status)
              if [ "$status" = "Playing" ]; then
                title=$(${lib.getExe pkgs.playerctl} metadata --player=spotify title 2>/dev/null)
                artist=$(${lib.getExe pkgs.playerctl} metadata --player=spotify artist 2>/dev/null)
                echo -e "<span font_size='32000' weight='bold'>$title</span>\\n<span font_size='25000' alpha='85%'>$artist</span>"
              fi
            '';
          in
          [
            {
              monitor = "";
              text = "cmd[update:10000] echo -e \"<span font_size='90000' weight='bold' line_height='0.8'>$(date +'%H:%M')</span>\\n<span font_size='25000' alpha='85%'>$(date +'%A, %d %B %Y')</span>\"";
              color = "rgba(226, 183, 20, 1)";
              font_size = 1;
              font_family = "JetBrainsMono Nerd Font";
              text_align = "left";
              position = "30, -25";
              halign = "left";
              valign = "top";
            }
            {
              monitor = "";
              text = "cmd[update:2000] ${playerctlMetadata}";
              color = "rgba(226, 183, 20, 1)";
              font_size = 1;
              font_family = "JetBrainsMono Nerd Font";
              text_align = "right";
              position = "-30, -30";
              halign = "right";
              valign = "top";
            }
          ];

        input-field = [
          {
            monitor = "";
            position = "30, 30";
            size = "400, 60";
            outline_thickness = 2;
            dots_size = 0.2;
            dots_spacing = 0.2;
            dots_center = true;
            outer_color = "rgba(243, 204, 38, 1)";
            inner_color = "rgba(57, 65, 71, 0.6)";
            fade_on_empty = false;
            placeholder_text = ''
              <span foreground="##f3cc26ff">󰌾 <span foreground="##cdd6f4">Logged in as <span foreground="##f3cc26ff">$USER</span> ($LAYOUT[en,ru])</span></span>
            '';
            font_family = "JetBrainsMono Nerd Font";
            hide_input = false;
            check_color = "rgba(205, 214, 244, 1)";
            fail_color = "rgba(243, 139, 168, 1)";
            fail_text = ''<i>$FAIL <b>($ATTEMPTS)</b></i>'';
            font_color = "rgba(226, 183, 20, 1)";
            capslock_color = "rgba(249, 226, 175, 1)";
            halign = "left";
            valign = "bottom";
          }
        ];
      };
    };
  };
}

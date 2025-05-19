{ pkgs, ... }:
{
  fonts.packages =
    with pkgs;
    builtins.filter lib.attrsets.isDerivation (builtins.attrValues nerd-fonts); # TODO: Strip to only useful fonts

  home-manager.users.elxreno = {
    programs.waybar = {
      enable = true;
      settings = {
        mainBar = {
          output = [
            "eDP-1"
            "HDMI-A-1"
          ];
          layer = "top";
          position = "top";
          modules-left = [ "hyprland/workspaces" ];
          modules-center = [ "hyprland/window" ];
          modules-right = [
            "wireplumber"
            "network"
            "cpu"
            "temperature"
            "memory"
            "power-profiles-daemon"
            "battery"
            "backlight"
            "hyprland/language"
            "clock"
            "tray"
          ];

          "hyprland/workspaces" = {
            on-click = "activate";
            sort-by-number = true;
          };

          "clock" = {
            format = "{:%H:%M}";
            format-alt = "{:%Y-%m-%d}";
            tooltip-format = "<tt><small>{calendar}</small></tt>";
            calendar = {
              mode = "year";
              mode-mon-col = 3;
              weeks-pos = "right";
              on-scroll = 1;
              format = {
                months = "<span color='#ffead3'><b>{}</b></span>";
                days = "<span color='#ecc6d9'><b>{}</b></span>";
                weeks = "<span color='#99ffdd'><b>W{}</b></span>";
                weekdays = "<span color='#ffcc66'><b>{}</b></span>";
                today = "<span color='#ff6699'><b><u>{}</u></b></span>";
              };
            };
            actions = {
              "on-click-right" = "mode";
              # "on-scroll-up" = "tz_up";
              # "on-scroll-down" = "tz_down";
              "on-scroll-up" = "shift_up";
              "on-scroll-down" = "shift_down";
            };
          };

          "cpu" = {
            interval = 2;
            format = "  {usage}%";
          };

          "temperature" = {
            hwmon-path = "/sys/devices/pci0000:00/0000:00:18.3/hwmon/hwmon5/temp1_input";
            critical-threshold = 80;
            interval = 2;
            format = "{icon} {temperatureC}C°";
            format-icons = [ "" ];
          };

          "memory" = {
            format = "  {percentage}%";
            interval = 2;
          };

          "power-profiles-daemon" = {
            format = "{icon} {profile}";
            format-icons = {
              default = "-";
              performance = "";
              balanced = " ";
              power-saver = " ";
            };
          };

          "network" = {
            format-wifi = "  {bandwidthDownBytes}   {bandwidthUpBytes}";
            format-ethernet = "  {bandwidthDownBytes}   {bandwidthUpBytes}";
            format-linked = " (No IP)";
            format-disconnected = "";
            tooltip-format = "{ifname} via {gwaddr}";
            interval = 2;
          };

          "wireplumber" = {
            format = "{icon} {volume}%";
            format-muted = " Off";
            on-click = "pavucontrol";
            format-icons = [
              ""
              ""
              ""
            ];
          };

          "backlight" = {
            format = "{icon} {percent}%";
            format-icons = [
              ""
              ""
              ""
              ""
              ""
              ""
              ""
              ""
              ""
            ];
          };

          "battery" = {
            interval = 2;

            states = {
              good = 80;
              warning = 30;
              critical = 15;
            };
            format = "{icon} {capacity}% {power:2.1f}W";
            format-charging = " {capacity}% {power:2.1f}W";
            format-plugged = " {capacity}% {power:2.1f}W";
            format-alt = "{icon} {time}";
            format-icons = [
              " "
              " "
              " "
              " "
              " "
            ];
          };

          tray = {
            icon-size = 16;
            spacing = 10;
          };

          "hyprland/language" = {
            format = "  {}";
            format-en = "EN";
            format-ru = "RU";
          };
        };
      };

      style = ''
        * {
          border: none;
          border-radius: 0;
          font-family: "Terminess Nerd Font";
          font-size: 13px;
          min-height: 0;
        }

        window#waybar {
          background-color: rgba(26, 27, 38, 0.8);
          color: #ffffff;
        }

        #workspaces button {
          padding: 0 5px;
          color: #ffffff;
        }

        #workspaces button.active {
          background-color: #64727D;
          box-shadow: inset 0 -3px #ffffff;
        }

        #clock,
        #battery,
        #cpu,
        #temperature,
        #power-profiles-daemon,
        #language,
        #memory,
        #network,
        #backlight,
        #wireplumber {
          background-color: rgb(78, 112, 137);
          border-radius: 20px;
          padding: 0 5px;
          margin: 2px;
        }

        #tray {
          margin: 0 5px;
        }

        #battery.critical:not(.charging) {
          background-color: #f53c3c;
          color: #ffffff;
          animation-name: blink;
          animation-duration: 0.5s;
          animation-timing-function: linear;
          animation-iteration-count: infinite;
          animation-direction: alternate;
        }

        @keyframes blink {
          to {
            background-color: #ffffff;
            color: #000000;
          }
        }
      '';
    };
  };
}

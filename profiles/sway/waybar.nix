{ pkgs, ... }:

{
  home-manager.users.elxreno.programs.waybar = {
    enable = true;
    style = ''
      * {
        border: none;
        font-family: FiraCode Nerd Font Mono;
        font-weight: bold;
      }
      window#waybar {
        background-color: #1a1b26;
        /*background: transparent;*/
        transition-property: background-color;
        transition-duration: .5s;
        border-bottom: none;
      }
      window#waybar.hidden {
        opacity: 0.2;
      }
      #workspace,
      #mode,
      #clock,
      #pulseaudio,
      #network,
      #mpd,
      #memory,
      #network,
      #window,
      #cpu,
      #temperature,
      #backlight,
      #battery,
      #tray {
        background-color: #252734;
        padding: 0 12px;
        margin: 4px 4px 4px 4px;
        border-radius: 90px;
        background-clip: padding-box;
      }
      #workspaces button {
        padding: 0 5px;
        color: #7aa2f7;
        min-width: 20px;
      }
      #workspaces button:hover {
        background-color: rgba(0, 0, 0, 0.4);
      }
      #workspaces button.focused {
        color: #d19a66;
      }
      #workspaces button.urgent {
        color: #e06c75;
      }
      #language {
        color: #fff;
        padding: 0 12px;
      }
      #mode {
        color: #e06c75;
      }
      #cpu {
        color: #d19a66;
      }
      #temperature {
        color: #638ABF;
      }
      #memory {
        color: #c678dd;
      }
      #clock {
        color: #7aa2f7;
      }
      #window {
        color: #9ece6a;
        font-size: 13px;
      }
      #backlight {
        color: #E36A00;
      }
      #battery {
        color: #9ece6a;
      }
      #battery.warning {
        color: #ff5d17;
      }
      #battery.critical {
        color: #ff200c;
      }
      #battery.charging {
        color: #9ece6a;
      }
      #network {
        color: #c678dd;
      }
      #pulseaudio {
        color: #56b6c2;
      }
      #pulseaudio.muted {
        color: #c678dd;
        background-color: #252734;
      }
    '';
    settings = [{
      bar_id = "bar-0";
      ipc = true;
      layer = "top";
      position = "top";
      height = 35;
      tray = {
        icon-size = 14;
        spacing = 10;
        show-passive-items = true;
      };
      modules-center = [ "clock" ];
      modules-left = [ "sway/workspaces" ];
      modules-right = [
        "tray"
        "cpu"
        "temperature"
        "memory"
        "pulseaudio"
        "backlight"
        "battery"
        "network"
        "sway/language"
      ];
      clock = {
        format = "{:%b %d %H:%M}";
        tooltip-format = ''
          <big>{:%Y %B}</big>
          <tt><small>{calendar}</small></tt>'';
        format-alt = "{:%A, %B %d, %Y} ";
      };
      cpu = {
        format = "{usage}% <span font='14'></span>";
        tooltip = false;
        interval = 1;
      };
      temperature = {
        thermal-zone = 5;
        hwmon-path = "/sys/class/hwmon/hwmon5/temp1_input";
        critical-threshold = 80;
        format = "{temperatureC}°C ";
        interval = 5;
      };
      memory = {
        format = "{}% <span font='14'></span>";
        interval = 1;
      };
      backlight = {
        format = "{percent}% {icon}";
        format-icons = [ "" "" "" "" "" "" "" "" "" ];
      };
      battery = {
        bat = "BAT1";
        interval = 60;
        states = {
          warning = 30;
          critical = 15;
        };
        format = "{capacity}% <span font='16'>{icon}</span>";
        format-charging = "{capacity}% <span font='14'></span>";
        format-icons = [ "" "" "" "" "" ];
        max-length = 25;
      };
      network = {
        format-wifi = "{essid} <span font='14'></span>";
        format-ethernet =
          "<span font='14'></span> {ifname}: {ipaddr}/{cidr}";
        format-linked = "<span font='14'>睊</span> {ifname} (No IP)";
        format-disconnected = "<span font='14'>睊</span> Not connected";
        format-alt = "{ifname}: {ipaddr}/{cidr}";
        tooltip-format = "{essid} {signalStrength}%";
        on-click-right = "${pkgs.alacritty}/bin/alacritty -e nmtui";
      };
      pulseaudio = {
        format = "<span font='14'>{icon}</span> {volume}% {format_source}";
        format-bluetooth =
          "<span font='14'>{icon}</span> {volume}% {format_source}";
        format-bluetooth-muted =
          "<span font='14'></span> {volume}% {format_source}";
        format-muted = "<span font='13'></span> {format_source}";
        format-source = "{volume}% <span font='11'></span>";
        format-source-muted = "<span font='11'></span>";
        format-icons = {
          default = [ "" "" "" ];
          headphone = "";
          hands-free = "";
          headset = "";
          phone = "";
          portable = "";
          car = "";
        };
        tooltip-format = "{desc}, {volume}%";
        on-click = "${pkgs.pamixer}/bin/pamixer -t";
        on-click-right = "${pkgs.pamixer}/bin/pamixer --default-source -t";
        on-click-middle = "${pkgs.pavucontrol}/bin/pavucontrol";
      };
    }];
  };
}

{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.waybar;

  # https://github.com/Alexays/Waybar/issues/2162#issuecomment-1537366689
  mkBandwidthModule =
    type:
    pkgs.writeScript "bandwidth-module-${type}" ''
      #!${pkgs.python3}/bin/python3

      import subprocess
      from time import sleep


      def get_network_bytes(iface):
          with open("/proc/net/dev") as f:
              for line in f:
                  if not line.strip().startswith(f"{iface}:"):
                      continue
                  return line
          raise RuntimeError("Interface not found")

      def get_bytes(iface):
          network = get_network_bytes(iface).split()
          bytes = int(network[${if type == "rx" then "1" else "9"}])
          return bytes

      def format_speed(bytes_per_sec, width=5):
          mib = bytes_per_sec / (1024 * 1024)
          return f"{mib:>{width}.1f} MiB"

      def get_default_iface():
          # /proc/net/route: default route has Destination=00000000 and RTF_GATEWAY (0x2) in Flags
          with open("/proc/net/route") as f:
              next(f, None)  # skip header
              for line in f:
                  parts = line.strip().split()
                  if len(parts) >= 4:
                      iface, dest, flags_hex = parts[0], parts[1], parts[3]
                      flags = int(flags_hex, 16)
                      if dest == "00000000" and (flags & 0x2):
                          return iface
          raise RuntimeError("Default route not found")

      def main():
          refresh_interval = 2
          rx_icon = ""
          tx_icon = ""

          fmt_str = (
              f"{${type}_icon}{{${type}}}"
          )

          iface = get_default_iface()
          bytes = get_bytes(iface)

          while True:
              new_iface = get_default_iface()
              if new_iface != iface:
                  iface = new_iface
                  bytes = get_bytes(iface)
                  dbytes = 0
              else:
                  prev_bytes = bytes
                  bytes = get_bytes(iface)
                  dbytes = (bytes - prev_bytes) / refresh_interval

              dbytes_fmt = format_speed(dbytes)

              line = fmt_str.format(${type}=dbytes_fmt)
              print(line, flush=True)
              sleep(refresh_interval)


      if __name__ == "__main__":
          main()
    '';
in
{
  options.${namespace}.programs.waybar = {
    enable = mkEnableOption "Whether or not to manage waybar.";
  };

  config = mkIf cfg.enable {
    programs.waybar = {
      enable = true;
      systemd.enable = true;
      settings = {
        topBar = {
          output = [
            "*"
          ];
          layer = "top";
          position = "top";
          modules-left = [ "hyprland/workspaces" ];
          modules-center = [ "hyprland/window" ];
          modules-right = [
            "mpris"
          ];

          "hyprland/workspaces" = {
            on-click = "activate";
            sort-by-number = true;
          };

          "hyprland/window" = {
            separate-outputs = true;
          };

          "mpris" = {
            ignored-players = [ "firefox" ];
            dynamic-order = [
              "title"
              "artist"
            ];
          };
        };
        statusBar = {
          output = [
            "*"
          ];
          layer = "bottom";
          position = "bottom";
          modules-left = [ "tray" ];
          modules-center = [ "clock" ];
          modules-right = [
            "custom/bandwidth-rx"
            "custom/bandwidth-tx"
            "cpu"
            "temperature"
            "memory"
            "power-profiles-daemon"
            "battery"
            "backlight"
            "wireplumber"
            "hyprland/language"
            "idle_inhibitor"
          ];

          "custom/bandwidth-rx" = {
            exec = mkBandwidthModule "rx";
          };

          "custom/bandwidth-tx" = {
            exec = mkBandwidthModule "tx";
          };

          "cpu" = {
            interval = 2;
            format = " {usage}%";
          };

          "temperature" = {
            hwmon-path = "/sys/devices/pci0000:00/0000:00:18.3/hwmon/hwmon5/temp1_input";
            critical-threshold = 80;
            interval = 2;
            format = "{icon} {temperatureC}°C";
            format-icons = [ "" ];
          };

          "memory" = {
            format = " {percentage}%";
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
            format-wifi = " {bandwidthDownBytes}  {bandwidthUpBytes}";
            format-ethernet = " {bandwidthDownBytes}  {bandwidthUpBytes}";
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
              ""
              ""
              ""
              ""
              ""
            ];
          };

          tray = {
            icon-size = 16;
            spacing = 10;
          };

          "clock" = {
            format = "{:%H:%M - %Y-%m-%d}";
            # format-alt = "{:%Y-%m-%d}";
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

          "hyprland/language" = {
            format = " {}";
            format-en = "EN";
            format-ru = "RU";
          };

          "idle_inhibitor" = {
            format = "{icon}  ";
            format-icons = {
              activated = "";
              deactivated = "";
            };
          };
        };
      };

      style = ''
        * {
          border: none;
          border-radius: 0;
          font-family: "JetBrainsMono Nerd Font";
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

        #battery,
        #cpu,
        #temperature,
        #power-profiles-daemon,
        #language,
        #memory,
        #custom-bandwidth-rx,
        #custom-bandwidth-tx,
        #backlight,
        #wireplumber {
          background-color: #727f8b;
          border-radius: 20px;
          padding: 0 5px;
          margin: 2px;
        }

        #tray {
          margin: 0 5px;
        }

        #mpris,
        #language {
          margin-right: 10px;
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

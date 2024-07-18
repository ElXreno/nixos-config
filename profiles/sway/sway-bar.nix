{ pkgs, ... }:
let barPosition = "top";
in {
  home.packages = with pkgs; [ lm_sensors ];

  programs.i3status-rust = {
    enable = true;
    bars.${barPosition} = {
      blocks = [
        {
          block = "bluetooth";
          mac = "C0:D1:93:F6:94:6B";
          format = " $icon {$percentage|} ";
          disconnected_format = " $icon  ";
        }
        {
          block = "net";
          device = "(en|wl).*";
          format = " $icon  {$ssid $frequency} ";
          format_alt = " $icon{ $ssid $signal_strength|} ";
        }
        {
          block = "cpu";
          format = " $icon $utilization$frequency ";
        }
        {
          block = "temperature";
          format = " $icon $max ";
        }
        {
          block = "memory";
          format =
            " $icon $mem_used.eng(prefix:M)/$mem_total.eng(prefix:M)($mem_used_percents.eng(w:2)) ";
          format_alt =
            " $icon_swap $swap_used.eng(prefix:M)/$swap_total.eng(prefix:M)($swap_used_percents.eng(w:2)) ";
        }
        {
          block = "battery";
          format = " $icon  $percentage ";
          not_charging_format = " $icon  $percentage ";
        }
        { block = "backlight"; }
        {
          block = "sound";
          format = " $icon $output_name{ $volume|} ";
          mappings = {
            "alsa_output.pci-0000_03_00.6.analog-stereo" = "Speakers";
            "bluez_output.C0_D1_93_F6_94_6B.1" = "BT";
          };
        }
        {
          block = "time";
          format = " $timestamp.datetime(f:'%a %F %R') ";
        }
        {
          block = "keyboard_layout";
          driver = "sway";
          mappings = {
            "English (US)" = "EN";
            "Russian (N/A)" = "RU";
          };
        }
      ];
      icons = "awesome6";
    };
  };

  wayland.windowManager.sway.config.bars = [{
    position = barPosition;
    statusCommand =
      "${pkgs.i3status-rust}/bin/i3status-rs ~/.config/i3status-rust/config-${barPosition}.toml";
    fonts = {
      names = [ "FiraCode Nerd Font" ];
      size = 9.0;
    };
  }];
}
